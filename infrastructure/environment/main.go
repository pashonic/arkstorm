package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/batch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/kms"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/secretsmanager"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	weatherbell_username_var_name = "weatherbell-username"
	weatherbell_password_var_name = "weatherbell-password"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		// Config setup
		config := config.New(ctx, "")

		//Get resource names from boilerplate
		boilerPlateStack, err := pulumi.NewStackReference(ctx, config.Require("boilerplatestack"), nil)
		if err != nil {
			return err
		}

		// Get objects from boilerplate
		//dockerRepoUrl := boilerPlateStack.GetOutput(pulumi.String("docker-repo-url"))
		privateSubnetId := boilerPlateStack.GetStringOutput(pulumi.String("private-subnet"))
		securityGroupId := boilerPlateStack.GetStringOutput(pulumi.String("security-group"))
		//vpncId := boilerPlateStack.GetOutput(pulumi.String("vpc-id"))

		//
		// Access objects
		//

		kmsKey, err := kms.NewKey(ctx, "encrypter", &kms.KeyArgs{
			Description: pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			return err
		}
		secrets, err := secretsmanager.NewSecret(ctx, fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack()), &secretsmanager.SecretArgs{
			KmsKeyId: kmsKey.ID(),
		})
		if err != nil {
			return err
		}

		//
		// Storage pulumi secrets into Secrets Manager, layer 2 process
		//

		pulumi.All(secrets.Name, config.RequireSecret(weatherbell_username_var_name), config.RequireSecret(weatherbell_password_var_name)).ApplyT(
			func(args []interface{}) *pulumi.Output {
				secretId := args[0].(string)
				username := args[1].(string)
				password := args[2].(string)
				jsonBytes, err := json.Marshal(map[string]string{weatherbell_username_var_name: username, weatherbell_password_var_name: password})
				if err != nil {
					log.Fatalln(err)
				}
				_, err = secretsmanager.NewSecretVersion(ctx, "weatherbell-creds", &secretsmanager.SecretVersionArgs{
					SecretId:     pulumi.String(secretId),
					SecretString: pulumi.String(jsonBytes),
				})
				if err != nil {
					log.Fatalln(err)
				}
				return nil
			})

		//
		// Config file bucket
		//
		//configBucket, err := s3.NewBucket(ctx, "config", &s3.BucketArgs{
		_, err = s3.NewBucket(ctx, "config", &s3.BucketArgs{
			BucketPrefix: pulumi.String(fmt.Sprintf("%s-config-%s-", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			log.Fatal(err)
		}

		//
		// Youtube credentials bucket
		//

		//credsBucket, err := s3.NewBucket(ctx, "creds", &s3.BucketArgs{
		_, err = s3.NewBucket(ctx, "creds", &s3.BucketArgs{
			BucketPrefix: pulumi.String(fmt.Sprintf("%s-creds-%s-", ctx.Project(), ctx.Stack())),
			ServerSideEncryptionConfiguration: &s3.BucketServerSideEncryptionConfigurationArgs{
				Rule: &s3.BucketServerSideEncryptionConfigurationRuleArgs{
					ApplyServerSideEncryptionByDefault: &s3.BucketServerSideEncryptionConfigurationRuleApplyServerSideEncryptionByDefaultArgs{
						KmsMasterKeyId: kmsKey.Arn,
						SseAlgorithm:   pulumi.String("aws:kms"),
					},
				},
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		//
		// Create batch execute role
		//

		ecsAssumeRole, err := json.Marshal(map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": "sts:AssumeRole",
					"Principal": map[string]interface{}{
						"Service": "ecs-tasks.amazonaws.com",
					},
					"Effect": "Allow",
					"Sid":    "",
				},
			},
		})
		executeRole, err := iam.NewRole(ctx, "execute", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(ecsAssumeRole),
			NamePrefix:       pulumi.String(fmt.Sprintf("%s-%s-execute-", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			return err
		}
		_, err = iam.NewRolePolicyAttachment(ctx, "execute-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      executeRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		})
		if err != nil {
			return err
		}

		//
		// Create batch job context role (the container access context)
		//

		jobRole, err := iam.NewRole(ctx, "job", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(ecsAssumeRole),
			NamePrefix:       pulumi.String(fmt.Sprintf("%s-%s-job-", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			return err
		}
		_, err = iam.NewRolePolicyAttachment(ctx, "job-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      jobRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		})
		if err != nil {
			return err
		}

		//
		// Create batch compute role
		//

		computeRole, err := iam.NewServiceLinkedRole(ctx, "compute", &iam.ServiceLinkedRoleArgs{
			AwsServiceName: pulumi.String("batch.amazonaws.com"),
		})
		if err != nil {
			log.Fatal(err)
		}

		computeEnvArn := pulumi.All(securityGroupId, privateSubnetId).ApplyT(
			func(args []interface{}) (pulumi.StringOutput, error) {
				securityGroupId := args[0].(string)
				subnetId := args[1].(string)
				computeEnv, err := batch.NewComputeEnvironment(ctx, "compute", &batch.ComputeEnvironmentArgs{
					ComputeEnvironmentName: pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
					ComputeResources: &batch.ComputeEnvironmentComputeResourcesArgs{
						MaxVcpus: pulumi.Int(1),
						SecurityGroupIds: pulumi.StringArray{
							pulumi.String(securityGroupId),
						},
						Subnets: pulumi.StringArray{
							pulumi.String(subnetId),
						},
						Type: pulumi.String("FARGATE_SPOT"),
					},
					ServiceRole: computeRole.Arn,
					Type:        pulumi.String("MANAGED"),
				}, pulumi.DependsOn([]pulumi.Resource{
					computeRole,
				}))
				if err != nil {
					log.Fatal(err)
				}
				return computeEnv.Arn, nil
			}).(pulumi.StringOutput)

		_, err = batch.NewJobQueue(ctx, "jobqueue", &batch.JobQueueArgs{
			State:    pulumi.String("ENABLED"),
			Priority: pulumi.Int(1),
			Name:     pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
			ComputeEnvironments: pulumi.StringArray{
				computeEnvArn,
			},
		})
		if err != nil {
			return err
		}

		return nil
	})
}
