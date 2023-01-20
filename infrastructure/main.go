package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/batch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/kms"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/secretsmanager"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		awsConf := config.New(ctx, "aws")
		userConf := config.New(ctx, "")
		region := awsConf.Require("region")

		//
		// Access objects
		//

		kmsKey, err := kms.NewKey(ctx, "encrypter", &kms.KeyArgs{
			Description: pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			log.Fatal(err)
		}

		secrets, err := secretsmanager.NewSecret(ctx, fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack()), &secretsmanager.SecretArgs{
			KmsKeyId: kmsKey.ID(),
		})
		if err != nil {
			log.Fatal(err)
		}

		pulumi.All(secrets.Name, userConf.RequireSecret("weatherbell-username"), userConf.RequireSecret("weatherbell-password")).ApplyT(
			func(args []interface{}) *secretsmanager.SecretVersion {
				secretId := args[0].(string)
				username := args[1].(string)
				password := args[2].(string)
				jsonBytes, _ := json.Marshal(map[string]string{"weatherbell-username": username, "weatherbell-password": password})
				secretVersion, err := secretsmanager.NewSecretVersion(ctx, "weatherbell-creds", &secretsmanager.SecretVersionArgs{
					SecretId:     pulumi.String(secretId),
					SecretString: pulumi.String(jsonBytes),
				})
				if err != nil {
					log.Fatal(err)
				}
				return secretVersion
			})

		//
		// Buckets resources
		//

		configBucket, err := s3.NewBucket(ctx, "config", &s3.BucketArgs{
			BucketPrefix: pulumi.String(fmt.Sprintf("%s-config-%s-", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			log.Fatal(err)
		}

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
			log.Fatal(err)
		}

		//
		// Networking resources
		//

		vpc, err := ec2.NewVpc(ctx, "main", &ec2.VpcArgs{
			CidrBlock: pulumi.String("10.0.0.0/16"),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		securityGroup, err := ec2.NewDefaultSecurityGroup(ctx, "vpc-default", &ec2.DefaultSecurityGroupArgs{
			VpcId:   vpc.ID(),
			Ingress: ec2.DefaultSecurityGroupIngressArray{},
			Egress: ec2.DefaultSecurityGroupEgressArray{
				&ec2.DefaultSecurityGroupEgressArgs{
					FromPort: pulumi.Int(0),
					ToPort:   pulumi.Int(0),
					Protocol: pulumi.String("-1"),
					CidrBlocks: pulumi.StringArray{
						pulumi.String("0.0.0.0/0"),
					},
				},
			},
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		internetGateway, err := ec2.NewInternetGateway(ctx, "internetGateway", &ec2.InternetGatewayArgs{
			VpcId: vpc.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		subnetPublic, err := ec2.NewSubnet(ctx, "public", &ec2.SubnetArgs{
			VpcId:     vpc.ID(),
			CidrBlock: pulumi.String("10.0.0.0/20"),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-%s-public", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		subnetPrivate, err := ec2.NewSubnet(ctx, "private", &ec2.SubnetArgs{
			VpcId:     vpc.ID(),
			CidrBlock: pulumi.String("10.0.128.0/20"),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-%s-private", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		elasticIp, err := ec2.NewEip(ctx, "elasticip", &ec2.EipArgs{
			Vpc: pulumi.Bool(true),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-%s-%s", ctx.Project(), ctx.Stack(), region)),
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		natGateway, err := ec2.NewNatGateway(ctx, "nat", &ec2.NatGatewayArgs{
			AllocationId: elasticIp.ID(),
			SubnetId:     subnetPublic.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
			},
		}, pulumi.DependsOn([]pulumi.Resource{
			internetGateway,
		}))
		if err != nil {
			log.Fatal(err)
		}

		routePublic, err := ec2.NewRouteTable(ctx, "public", &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock: pulumi.String("0.0.0.0/0"),
					GatewayId: internetGateway.ID(),
				},
			},
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-%s-public", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		routePrivate, err := ec2.NewRouteTable(ctx, "private", &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock:    pulumi.String("0.0.0.0/0"),
					NatGatewayId: natGateway.ID(),
				},
			},
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-%s-private", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		_, err = ec2.NewRouteTableAssociation(ctx, "rta-Public", &ec2.RouteTableAssociationArgs{
			SubnetId:     subnetPublic.ID(),
			RouteTableId: routePublic.ID(),
		})
		if err != nil {
			log.Fatal(err)
		}

		_, err = ec2.NewRouteTableAssociation(ctx, "rta-Private", &ec2.RouteTableAssociationArgs{
			SubnetId:     subnetPrivate.ID(),
			RouteTableId: routePrivate.ID(),
		})
		if err != nil {
			log.Fatal(err)
		}

		//
		// ECR Repository
		//

		dockerRepo, err := ecr.NewRepository(ctx, "docker-repo", &ecr.RepositoryArgs{
			ImageScanningConfiguration: &ecr.RepositoryImageScanningConfigurationArgs{
				ScanOnPush: pulumi.Bool(false),
			},
			ImageTagMutability: pulumi.String("MUTABLE"),
			Name:               pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			log.Fatal(err)
		}

		//
		// Iam resources
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
			NamePrefix:       pulumi.String(fmt.Sprintf("%s-%s-execute", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			log.Fatal(err)
		}
		_, err = iam.NewRolePolicyAttachment(ctx, "execute-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      executeRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		})
		if err != nil {
			log.Fatal(err)
		}

		jobRole, err := iam.NewRole(ctx, "job", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(ecsAssumeRole),
			NamePrefix:       pulumi.String(fmt.Sprintf("%s-%s-job", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			log.Fatal(err)
		}
		_, err = iam.NewRolePolicyAttachment(ctx, "job-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      jobRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		})
		if err != nil {
			log.Fatal(err)
		}

		computeRole, err := iam.NewServiceLinkedRole(ctx, "compute", &iam.ServiceLinkedRoleArgs{
			AwsServiceName: pulumi.String("batch.amazonaws.com"),
		})
		if err != nil {
			log.Fatal(err)
		}

		//
		// Batch resources
		//

		computeEnv, err := batch.NewComputeEnvironment(ctx, "compute", &batch.ComputeEnvironmentArgs{
			ComputeEnvironmentName: pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
			ComputeResources: &batch.ComputeEnvironmentComputeResourcesArgs{
				MaxVcpus: pulumi.Int(1),
				SecurityGroupIds: pulumi.StringArray{
					securityGroup.ID(),
				},
				Subnets: pulumi.StringArray{
					subnetPrivate.ID(),
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

		jobQueue, err := batch.NewJobQueue(ctx, "jobqueue", &batch.JobQueueArgs{
			State:    pulumi.String("ENABLED"),
			Priority: pulumi.Int(1),
			Name:     pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
			ComputeEnvironments: pulumi.StringArray{
				computeEnv.Arn,
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		//
		// Create job definition
		//

		pulumi.All(jobRole.Arn, executeRole.Arn, dockerRepo.RepositoryUrl, secrets.Arn, credsBucket.Bucket, configBucket.Bucket, jobQueue.Arn).ApplyT(
			func(args []interface{}) *pulumi.Output {
				jobRoleArn := args[0].(string)
				executeRoleArn := args[1].(string)
				dockerRepoUrl := args[2].(string)
				secretsArn := args[3].(string)
				credsBucket := args[4].(string)
				configBucket := args[5].(string)
				jobQueueArn := args[6].(string)

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
							"value": fmt.Sprintf("s3://%s", credsBucket),
						},
						map[string]interface{}{
							"name":  "AWS_S3_BUCKET_CONFIG_FILE",
							"value": fmt.Sprintf("s3://%s/config.toml", configBucket),
						},
					},
				})
				if err != nil {
					log.Fatal(err)
				}

				jobDefinition, err := batch.NewJobDefinition(ctx, "jobdefinition", &batch.JobDefinitionArgs{
					PlatformCapabilities: pulumi.StringArray{
						pulumi.String("FARGATE"),
					},
					ContainerProperties: pulumi.String(jobDefContainerProperties),
					Type:                pulumi.String("container"),
					Name:                pulumi.String(fmt.Sprintf("%s-%s", ctx.Project(), ctx.Stack())),
				})
				if err != nil {
					log.Fatal(err)
				}

				pulumi.All(jobDefinition.Arn, jobQueueArn).ApplyT(
					func(args []interface{}) *pulumi.Output {
						jobDefinitionArn := args[0].(string)
						jobQueueArn := args[1].(string)

						eventPolicyData, _ := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
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

						eventRole, err := iam.NewRole(ctx, "event", &iam.RoleArgs{
							AssumeRolePolicy: pulumi.String(eventAssumeRole),
							NamePrefix:       pulumi.String(fmt.Sprintf("%s-%s-event", ctx.Project(), ctx.Stack())),
							InlinePolicies: iam.RoleInlinePolicyArray{
								&iam.RoleInlinePolicyArgs{
									Name:   pulumi.String("event-policy"),
									Policy: pulumi.String(eventPolicyData.Json),
								},
							},
						})
						if err != nil {
							log.Fatal(err)
						}

						eventRule, err := cloudwatch.NewEventRule(ctx, "myeventrule", &cloudwatch.EventRuleArgs{
							ScheduleExpression: pulumi.String("cron(0 */12 * * ? *)"),
							IsEnabled:          pulumi.Bool(false),
						})
						if err != nil {
							log.Fatal(err)
						}

						_, err = cloudwatch.NewEventTarget(ctx, "myeventtarget", &cloudwatch.EventTargetArgs{
							Rule: eventRule.Name,
							BatchTarget: &cloudwatch.EventTargetBatchTargetArgs{
								JobDefinition: pulumi.String(jobDefinitionArn),
								JobName:       pulumi.String(fmt.Sprintf("%s-%s-event", ctx.Project(), ctx.Stack())),
							},
							RoleArn: eventRole.Arn,
							Arn:     pulumi.String(jobQueueArn),
						})
						if err != nil {
							log.Fatal(err)
						}

						return nil

					})

				ctx.Export("Job Definition", jobDefinition.Name)
				return nil
			})

		//
		// Attach resource specific access to roles
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

				jobsPolicyData, _ := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
					Statements: []iam.GetPolicyDocumentStatement{
						{
							Actions: []string{
								"s3:GetObject",
								"kms:Decrypt",
								"s3:ListBucket",
							},
							Resources: []string{
								configBucketArn,
								configBucketArn + "/*",
								credsBucketArn,
								credsBucketArn + "/*",
								kmsKeyArn,
							},
						},
					},
				}, nil)

				//
				// Job role access
				//

				jobPolicy, err := iam.NewPolicy(ctx, "job-resource-policy", &iam.PolicyArgs{
					Path:        pulumi.String("/"),
					Name:        pulumi.String(fmt.Sprintf("%s-%s-job", ctx.Project(), ctx.Stack())),
					Description: pulumi.String("S3 Access"),
					Policy:      pulumi.String(jobsPolicyData.Json),
				})
				if err != nil {
					log.Fatal(err)
				}
				_, err = iam.NewRolePolicyAttachment(ctx, "job-resource-attachment", &iam.RolePolicyAttachmentArgs{
					Role:      pulumi.String(jobRoleName),
					PolicyArn: jobPolicy.Arn,
				})
				if err != nil {
					log.Fatal(err)
				}

				//
				// Execute role access
				//

				executePolicyData, _ := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
					Statements: []iam.GetPolicyDocumentStatement{
						{
							Actions: []string{
								"kms:Decrypt",
								"secretsmanager:GetSecretValue",
							},
							Resources: []string{
								secretsArn,
								kmsKeyArn,
							},
						},
					},
				}, nil)
				executePolicy, err := iam.NewPolicy(ctx, "execute-resource-policy", &iam.PolicyArgs{
					Path:        pulumi.String("/"),
					Name:        pulumi.String(fmt.Sprintf("%s-%s-execute", ctx.Project(), ctx.Stack())),
					Description: pulumi.String("Secrets Manager Access"),
					Policy:      pulumi.String(executePolicyData.Json),
				})
				if err != nil {
					log.Fatal(err)
				}
				_, err = iam.NewRolePolicyAttachment(ctx, "execute-resource-attachment", &iam.RolePolicyAttachmentArgs{
					Role:      pulumi.String(executeRoleName),
					PolicyArn: executePolicy.Arn,
				})
				if err != nil {
					log.Fatal(err)
				}

				return nil
			})

		ctx.Export("Docker Repo Url", dockerRepo.RepositoryUrl)
		ctx.Export("config Bucket", configBucket.Bucket)
		ctx.Export("creds Bucket", credsBucket.Bucket)

		return nil
	})
}
