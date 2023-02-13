package main

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/batch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/kms"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/secretsmanager"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/sns"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	weatherbell_username_var_name = "weatherbell-username"
	weatherbell_password_var_name = "weatherbell-password"
)

type Job struct {
	Name     string
	Schedule string
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		//
		// Config Setup
		//

		config := config.New(ctx, "")
		boilerPlateStack, err := pulumi.NewStackReference(ctx, config.Require("boilerplatestack"), nil)
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		dockerRepoUrl := boilerPlateStack.GetOutput(pulumi.String("docker-repo-url"))
		privateSubnetId := boilerPlateStack.GetStringOutput(pulumi.String("private-subnet"))
		securityGroupId := boilerPlateStack.GetStringOutput(pulumi.String("security-group"))

		//
		// Access objects
		//

		kmsKey, err := kms.NewKey(ctx, "encrypter", &kms.KeyArgs{
			Description: pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		secrets, err := secretsmanager.NewSecret(ctx, fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack()), &secretsmanager.SecretArgs{
			KmsKeyId: kmsKey.ID(),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
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
					ctx.Log.Error(err.Error(), nil)
				}
				_, err = secretsmanager.NewSecretVersion(ctx, "weatherbell-creds", &secretsmanager.SecretVersionArgs{
					SecretId:     pulumi.String(secretId),
					SecretString: pulumi.String(jsonBytes),
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}
				return nil
			})

		//
		// Config file bucket
		//

		configBucket, err := s3.NewBucket(ctx, "config", &s3.BucketArgs{
			BucketPrefix: pulumi.String(fmt.Sprintf("%s-config-%s-", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}

		//
		// Youtube credentials bucket
		//

		credsBucket, err := s3.NewBucket(ctx, "creds", &s3.BucketArgs{
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
			ctx.Log.Error(err.Error(), nil)
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
			ctx.Log.Error(err.Error(), nil)
		}
		_, err = iam.NewRolePolicyAttachment(ctx, "execute-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      executeRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}

		//
		// Create batch job context role (the container access context)
		//

		jobRole, err := iam.NewRole(ctx, "job", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(ecsAssumeRole),
			NamePrefix:       pulumi.String(fmt.Sprintf("%s-%s-job-", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		_, err = iam.NewRolePolicyAttachment(ctx, "job-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      jobRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}

		//
		// Create batch compute role
		//

		computeRole, err := iam.NewServiceLinkedRole(ctx, "compute", &iam.ServiceLinkedRoleArgs{
			AwsServiceName: pulumi.String("batch.amazonaws.com"),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
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
					ctx.Log.Error(err.Error(), nil)
				}
				return computeEnv.Arn, nil
			}).(pulumi.StringOutput)

		jobQueue, err := batch.NewJobQueue(ctx, "jobqueue", &batch.JobQueueArgs{
			State:    pulumi.String("ENABLED"),
			Priority: pulumi.Int(1),
			Name:     pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
			ComputeEnvironments: pulumi.StringArray{
				computeEnvArn,
			},
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}

		//
		// Create sns failure
		//

		snsFailure, err := sns.NewTopic(ctx, "failure", &sns.TopicArgs{
			NamePrefix: pulumi.String(fmt.Sprintf("%s-%s-failure-", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}

		//
		// Create failure alert pipe
		//

		pulumi.All(jobQueue.Arn, snsFailure.Arn).ApplyT(
			func(args []interface{}) *pulumi.Output {
				jobQueueArn := args[0].(string)
				snsFailureArn := args[1].(string)

				//
				// Create cloud watch event
				//

				emailEventPattern, err := json.Marshal(map[string]interface{}{
					"detail-type": []interface{}{
						"Batch Job State Change",
					},
					"source": []interface{}{
						"aws.batch",
					},
					"detail": map[string]interface{}{
						"status": []interface{}{
							"FAILED",
						},
						"jobQueue": []interface{}{
							jobQueueArn,
						},
					},
				})
				jobEmailEventRule, err := cloudwatch.NewEventRule(ctx, "sns-failure", &cloudwatch.EventRuleArgs{
					IsEnabled:    pulumi.Bool(true),
					NamePrefix:   pulumi.String(fmt.Sprintf("%s-%s-failure-", ctx.Project(), ctx.Stack())),
					EventPattern: pulumi.String(emailEventPattern),
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}

				//
				// Create lambda function to customize SNS topic/email
				//

				role, err := iam.NewRole(ctx, "sns-failure", &iam.RoleArgs{
					AssumeRolePolicy: pulumi.String(`{
						"Version": "2012-10-17",
						"Statement": [{
							"Sid": "",
							"Effect": "Allow",
							"Principal": {
								"Service": "lambda.amazonaws.com"
							},
							"Action": "sts:AssumeRole"
						}]
					}`),
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}

				executePolicyData, err := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
					Statements: []iam.GetPolicyDocumentStatement{
						{
							Actions: []string{
								"logs:CreateLogGroup",
								"logs:CreateLogStream",
								"logs:PutLogEvents",
							},
							Resources: []string{
								"*",
							},
						},
						{
							Actions: []string{
								"sns:Publish",
							},
							Resources: []string{
								snsFailureArn,
							},
						},
					},
				}, nil)
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}

				logPolicy, err := iam.NewRolePolicy(ctx, "lambda-log-policy", &iam.RolePolicyArgs{
					Role:   role.Name,
					Policy: pulumi.String(executePolicyData.Json),
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}

				assetArchive := pulumi.NewAssetArchive(map[string]interface{}{
					"lambda_function.py": pulumi.NewStringAsset(
						`import boto3
import json
import os
sns_arn = os.environ['SNS_TOPIC']

def lambda_handler(event, context):
	client = boto3.client("sns")
	job = event["detail"]["jobName"]  
	status = event["detail"]["status"]
	subject = "{0}-{1}".format(job,status)
	resp = client.publish(TargetArn=sns_arn, Message=json.dumps(event), Subject=subject)		 
`),
				})

				lambdaAlert, err := lambda.NewFunction(
					ctx,
					"alert-lambda",
					&lambda.FunctionArgs{
						Handler: pulumi.String("lambda_function.lambda_handler"),
						Role:    role.Arn,
						Runtime: pulumi.String("python3.9"),
						Code:    assetArchive,
						Name:    pulumi.String(fmt.Sprintf("%s-%s-alert", ctx.Project(), ctx.Stack())),
						Environment: &lambda.FunctionEnvironmentArgs{
							Variables: pulumi.StringMap{
								"SNS_TOPIC": pulumi.String(snsFailureArn),
							},
						},
					},
					pulumi.DependsOn([]pulumi.Resource{logPolicy}),
				)
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}

				_, err = lambda.NewPermission(ctx, "allowCloudwatch", &lambda.PermissionArgs{
					Action:    pulumi.String("lambda:InvokeFunction"),
					Function:  lambdaAlert.Name,
					Principal: pulumi.String("events.amazonaws.com"),
					SourceArn: jobEmailEventRule.Arn,
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}

				return nil
			},
		)

		//
		// Create job definition(s), layer 2 process
		//

		pulumi.All(jobRole.Arn, executeRole.Arn, dockerRepoUrl, secrets.Arn, credsBucket.Bucket, configBucket.Bucket, jobQueue.Arn).ApplyT(
			func(args []interface{}) *pulumi.Output {
				jobRoleArn := args[0].(string)
				executeRoleArn := args[1].(string)
				dockerRepoUrl := args[2].(string)
				secretsArn := args[3].(string)
				credsBucket := args[4].(string)
				configBucket := args[5].(string)
				jobQueueArn := args[6].(string)

				//
				// Get jobs configuration
				//

				jobs := []Job{}
				config.RequireObject("jobs", &jobs)

				//
				// Process jobs
				//

				for _, job := range jobs {

					//
					// Create SNS topic for emails
					//

					alert, err := sns.NewTopic(ctx, "alert"+job.Name, &sns.TopicArgs{
						NamePrefix: pulumi.String(fmt.Sprintf("%s-%s-%s-", ctx.Project(), ctx.Stack(), job.Name)),
					})
					if err != nil {
						ctx.Log.Error(err.Error(), nil)
					}
					ctx.Export("Alert Job SNS ARN for "+job.Name, alert.Arn)

					//
					// Define job confguration settings
					//

					jobDefContainerProperties, err := json.Marshal(map[string]interface{}{
						"image":            fmt.Sprintf("%s:latest", dockerRepoUrl),
						"jobRoleArn":       jobRoleArn,
						"executionRoleArn": executeRoleArn,
						"logConfiguration": map[string]interface{}{
							"logDriver": "awslogs",
						},
						"resourceRequirements": []interface{}{
							map[string]interface{}{
								"value": "1",
								"type":  "VCPU",
							},
							map[string]interface{}{
								"value": "2048",
								"type":  "MEMORY",
							},
						},
						"secrets": []interface{}{
							map[string]interface{}{
								"name":      "WEATHERBELL_USERNAME",
								"valueFrom": fmt.Sprintf("%s:weatherbell-username::", secretsArn),
							},
							map[string]interface{}{
								"name":      "WEATHERBELL_PASSWORD",
								"valueFrom": fmt.Sprintf("%s:weatherbell-password::", secretsArn),
							},
						},
						"environment": []interface{}{
							map[string]interface{}{
								"name":  "AWS_S3_CREDS_BUCKET",
								"value": fmt.Sprintf("s3://%s/%s", credsBucket, job.Name),
							},
							map[string]interface{}{
								"name":  "AWS_S3_BUCKET_CONFIG_FILE",
								"value": fmt.Sprintf("s3://%s/%s/active.toml", configBucket, job.Name),
							},
						},
					})
					if err != nil {
						ctx.Log.Error(err.Error(), nil)
					}

					jobDefinition, err := batch.NewJobDefinition(ctx, "jobdefinition-"+job.Name, &batch.JobDefinitionArgs{
						PlatformCapabilities: pulumi.StringArray{
							pulumi.String("FARGATE"),
						},
						ContainerProperties: pulumi.String(jobDefContainerProperties),
						Type:                pulumi.String("container"),
						Name:                pulumi.String(fmt.Sprintf("%s-%s-%s", ctx.Project(), ctx.Stack(), job.Name)),
					})
					if err != nil {
						ctx.Log.Error(err.Error(), nil)
					}
					ctx.Export(fmt.Sprintf("%s job definition", job.Name), jobDefinition.Name)

					//
					// Create scheduled event, layer 3 process
					//

					pulumi.All(jobDefinition.Arn, jobQueueArn, job.Name).ApplyT(
						func(args []interface{}) *pulumi.Output {
							jobDefinitionArn := args[0].(string)
							jobQueueArn := args[1].(string)
							jobName := args[2].(string)

							//
							// Create event role
							//

							eventPolicyData, err := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
								Statements: []iam.GetPolicyDocumentStatement{
									{
										Actions: []string{
											"batch:SubmitJob",
										},
										Resources: []string{
											jobDefinitionArn,
											jobQueueArn,
										},
									},
								},
							}, nil)
							if err != nil {
								ctx.Log.Error(err.Error(), nil)
							}
							eventAssumeRole, err := json.Marshal(map[string]interface{}{
								"Version": "2012-10-17",
								"Statement": []interface{}{
									map[string]interface{}{
										"Action": "sts:AssumeRole",
										"Principal": map[string]interface{}{
											"Service": "events.amazonaws.com",
										},
										"Effect": "Allow",
										"Sid":    "",
									},
								},
							})
							if err != nil {
								ctx.Log.Error(err.Error(), nil)
							}
							eventRole, err := iam.NewRole(ctx, "event-"+jobName, &iam.RoleArgs{
								AssumeRolePolicy: pulumi.String(eventAssumeRole),
								NamePrefix:       pulumi.String(fmt.Sprintf("%s-%s-event-", ctx.Project(), ctx.Stack())),
								InlinePolicies: iam.RoleInlinePolicyArray{
									&iam.RoleInlinePolicyArgs{
										Name:   pulumi.String("event-policy-" + jobName),
										Policy: pulumi.String(eventPolicyData.Json),
									},
								},
							})
							if err != nil {
								ctx.Log.Error(err.Error(), nil)
							}

							eventRule, err := cloudwatch.NewEventRule(ctx, "eventrule-"+jobName, &cloudwatch.EventRuleArgs{
								ScheduleExpression: pulumi.String(fmt.Sprintf("cron(%s)", job.Schedule)),
								IsEnabled:          pulumi.Bool(false),
								NamePrefix:         pulumi.String(fmt.Sprintf("%s-%s-%s-", ctx.Project(), ctx.Stack(), jobName)),
							})
							if err != nil {
								ctx.Log.Error(err.Error(), nil)
							}
							ctx.Export(fmt.Sprintf("%s event rule", jobName), eventRule.Name)
							_, err = cloudwatch.NewEventTarget(ctx, "eventtarget-"+jobName, &cloudwatch.EventTargetArgs{
								Rule: eventRule.Name,
								BatchTarget: &cloudwatch.EventTargetBatchTargetArgs{
									JobDefinition: pulumi.String(jobDefinitionArn),
									JobName:       pulumi.String(fmt.Sprintf("%s-%s-%s-event", ctx.Project(), ctx.Stack(), jobName)),
								},
								RoleArn: eventRole.Arn,
								Arn:     pulumi.String(jobQueueArn),
							})
							if err != nil {
								ctx.Log.Error(err.Error(), nil)
							}

							_, err = s3.NewBucketObject(ctx, "activefile-"+jobName, &s3.BucketObjectArgs{
								Key:     pulumi.String(fmt.Sprintf("%s/active.toml", jobName)),
								Bucket:  pulumi.String(configBucket),
								Content: pulumi.String("Replace Me"),
							})
							if err != nil {
								ctx.Log.Error(err.Error(), nil)
							}

							_, err = s3.NewBucketObject(ctx, "secretfile-"+jobName, &s3.BucketObjectArgs{
								Key:     pulumi.String(fmt.Sprintf("%s/client_secret.json", jobName)),
								Bucket:  pulumi.String(credsBucket),
								Content: pulumi.String("Replace Me"),
							})
							if err != nil {
								ctx.Log.Error(err.Error(), nil)
							}

							_, err = s3.NewBucketObject(ctx, "tokenfile-"+jobName, &s3.BucketObjectArgs{
								Key:     pulumi.String(fmt.Sprintf("%s/client_token.json", jobName)),
								Bucket:  pulumi.String(credsBucket),
								Content: pulumi.String("Replace Me"),
							})
							if err != nil {
								ctx.Log.Error(err.Error(), nil)
							}
							return nil
						})
				}
				return nil
			})

		//
		// Attach resource specific access to roles, layer 2 process
		//

		pulumi.All(configBucket.Arn, credsBucket.Arn, kmsKey.Arn, secrets.Arn, jobRole.Name, executeRole.Name).ApplyT(
			func(args []interface{}) *pulumi.Output {
				configBucketArn := args[0].(string)
				credsBucketArn := args[1].(string)
				kmsKeyArn := args[2].(string)
				secretsArn := args[3].(string)
				jobRoleName := args[4].(string)
				executeRoleName := args[5].(string)

				//
				// Job role access
				//

				jobsPolicyData, err := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
					Statements: []iam.GetPolicyDocumentStatement{
						{
							Actions: []string{
								"s3:GetObject",
								"s3:ListBucket",
							},
							Resources: []string{
								configBucketArn,
								configBucketArn + "/*",
								credsBucketArn,
								credsBucketArn + "/*",
							},
						},
						{
							Actions: []string{
								"kms:Decrypt",
							},
							Resources: []string{
								kmsKeyArn,
							},
						},
						{
							Actions: []string{
								"sns:Publish",
							},
							Resources: []string{
								"*",
							},
						},
					},
				}, nil)
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}
				jobPolicy, err := iam.NewPolicy(ctx, "job-resource-policy", &iam.PolicyArgs{
					Path:        pulumi.String("/"),
					Name:        pulumi.String(fmt.Sprintf("%s-%s-job", ctx.Project(), ctx.Stack())),
					Description: pulumi.String("S3 and KMS Access"),
					Policy:      pulumi.String(jobsPolicyData.Json),
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}
				_, err = iam.NewRolePolicyAttachment(ctx, "job-resource-attachment", &iam.RolePolicyAttachmentArgs{
					Role:      pulumi.String(jobRoleName),
					PolicyArn: jobPolicy.Arn,
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}

				//
				// Execute role access
				//

				executePolicyData, err := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
					Statements: []iam.GetPolicyDocumentStatement{
						{
							Actions: []string{
								"kms:Decrypt",
							},
							Resources: []string{
								kmsKeyArn,
							},
						},
						{
							Actions: []string{
								"secretsmanager:GetSecretValue",
							},
							Resources: []string{
								secretsArn,
							},
						},
					},
				}, nil)
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}
				executePolicy, err := iam.NewPolicy(ctx, "execute-resource-policy", &iam.PolicyArgs{
					Path:        pulumi.String("/"),
					Name:        pulumi.String(fmt.Sprintf("%s-%s-execute", ctx.Project(), ctx.Stack())),
					Description: pulumi.String("Secrets Manager and KMS Access"),
					Policy:      pulumi.String(executePolicyData.Json),
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}
				_, err = iam.NewRolePolicyAttachment(ctx, "execute-resource-attachment", &iam.RolePolicyAttachmentArgs{
					Role:      pulumi.String(executeRoleName),
					PolicyArn: executePolicy.Arn,
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}
				return nil
			})

		ctx.Export("Config Bucket", configBucket.Bucket)
		ctx.Export("Creds Bucket", credsBucket.Bucket)
		return nil
	})
}
