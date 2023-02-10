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
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/secretsmanager"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/sns"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type Job struct {
	Name     string
	Schedule string
}

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

		//
		// Storage pulumi secrets into Secrets Manager, layer 2 process
		//

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
		// Config file bucket
		//

		configBucket, err := s3.NewBucket(ctx, "config", &s3.BucketArgs{
			BucketPrefix: pulumi.String(fmt.Sprintf("%s-config-%s-", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			log.Fatal(err)
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
			log.Fatal(err)
		}
		_, err = iam.NewRolePolicyAttachment(ctx, "execute-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      executeRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		})
		if err != nil {
			log.Fatal(err)
		}

		//
		// Create batch job context role (the container access context)
		//

		jobRole, err := iam.NewRole(ctx, "job", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(ecsAssumeRole),
			NamePrefix:       pulumi.String(fmt.Sprintf("%s-%s-job-", ctx.Project(), ctx.Stack())),
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

		//
		// Create batch compute role
		//

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
		// Create alerting
		//

		snsAlert, err := sns.NewTopic(ctx, "alert", &sns.TopicArgs{
			NamePrefix: pulumi.String(fmt.Sprintf("%s-%s-alert-", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			return err
		}

		pulumi.All(jobQueue.Arn, snsAlert.Arn).ApplyT(
			func(args []interface{}) *pulumi.Output {
				jobQueueArn := args[0].(string)
				snsArn := args[1].(string)

				// Create event with batch job target
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
				jobEmailEventRule, err := cloudwatch.NewEventRule(ctx, "sns-alert", &cloudwatch.EventRuleArgs{
					IsEnabled:    pulumi.Bool(true),
					NamePrefix:   pulumi.String(fmt.Sprintf("%s-%s-alert-", ctx.Project(), ctx.Stack())),
					EventPattern: pulumi.String(emailEventPattern),
				})
				if err != nil {
					log.Fatal(err)
				}

				role, err := iam.NewRole(ctx, "sns-alert", &iam.RoleArgs{
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
					log.Fatal(err)
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
								snsArn,
							},
						},
					},
				}, nil)
				if err != nil {
					log.Fatal(err)
				}

				logPolicy, err := iam.NewRolePolicy(ctx, "lambda-log-policy", &iam.RolePolicyArgs{
					Role:   role.Name,
					Policy: pulumi.String(executePolicyData.Json),
				})
				if err != nil {
					log.Fatal(err)
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
								"SNS_TOPIC": pulumi.String(snsArn),
							},
						},
					},
					pulumi.DependsOn([]pulumi.Resource{logPolicy}),
				)
				if err != nil {
					log.Fatal(err)
				}

				_, err = lambda.NewPermission(ctx, "allowCloudwatch", &lambda.PermissionArgs{
					Action:    pulumi.String("lambda:InvokeFunction"),
					Function:  lambdaAlert.Name,
					Principal: pulumi.String("events.amazonaws.com"),
					SourceArn: jobEmailEventRule.Arn,
				})
				if err != nil {
					log.Fatal(err)
				}

				return nil
			},
		)

		//
		// Create job definition, layer 2 process
		//

		pulumi.All(jobRole.Arn, executeRole.Arn, dockerRepo.RepositoryUrl, secrets.Arn, credsBucket.Bucket, configBucket.Bucket, jobQueue.Arn, snsAlert.Arn).ApplyT(
			func(args []interface{}) *pulumi.Output {
				jobRoleArn := args[0].(string)
				executeRoleArn := args[1].(string)
				dockerRepoUrl := args[2].(string)
				secretsArn := args[3].(string)
				credsBucket := args[4].(string)
				configBucket := args[5].(string)
				jobQueueArn := args[6].(string)
				snsAlertArn := args[7].(string)

				// Get jobs configuration
				cfg := config.New(ctx, "")
				jobs := []Job{}
				cfg.RequireObject("jobs", &jobs)

				// Process jobs
				for _, job := range jobs {
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
							map[string]interface{}{
								"name":  "YOUTUBE_UPLOAD_ALERT_SNS_ARN",
								"value": snsAlertArn,
							},
						},
					})
					if err != nil {
						log.Fatal(err)
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
						log.Fatal(err)
					}

					// Create scheduled event, layer 3 process
					pulumi.All(jobDefinition.Arn, jobQueueArn, job.Name).ApplyT(
						func(args []interface{}) *pulumi.Output {
							jobDefinitionArn := args[0].(string)
							jobQueueArn := args[1].(string)
							jobName := args[2].(string)

							// Create event role
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
								log.Fatal(err)
							}

							// Create event with batch job target
							eventRule, err := cloudwatch.NewEventRule(ctx, "eventrule-"+jobName, &cloudwatch.EventRuleArgs{
								ScheduleExpression: pulumi.String(fmt.Sprintf("cron(%s)", userConf.Require("default-event-cron"))),
								IsEnabled:          pulumi.Bool(false),
								NamePrefix:         pulumi.String(fmt.Sprintf("%s-%s-%s-", ctx.Project(), ctx.Stack(), jobName)),
							})
							if err != nil {
								log.Fatal(err)
							}
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
								log.Fatal(err)
							}

							_, err = s3.NewBucketObject(ctx, "activefile-"+jobName, &s3.BucketObjectArgs{
								Key:     pulumi.String(fmt.Sprintf("%s/active.toml", jobName)),
								Bucket:  pulumi.String(configBucket),
								Content: pulumi.String("Replace Me"),
							})

							_, err = s3.NewBucketObject(ctx, "secretfile-"+jobName, &s3.BucketObjectArgs{
								Key:     pulumi.String(fmt.Sprintf("%s/client_secret.json", jobName)),
								Bucket:  pulumi.String(credsBucket),
								Content: pulumi.String("Replace Me"),
							})

							_, err = s3.NewBucketObject(ctx, "tokenfile-"+jobName, &s3.BucketObjectArgs{
								Key:     pulumi.String(fmt.Sprintf("%s/client_token.json", jobName)),
								Bucket:  pulumi.String(credsBucket),
								Content: pulumi.String("Replace Me"),
							})

							ctx.Export(fmt.Sprintf("%s job definition", jobName), jobDefinition.Name)

							return nil
						})
				}
				return nil
			})

		//
		// Attach resource specific access to roles, layer 2 process
		//

		pulumi.All(configBucket.Arn, credsBucket.Arn, kmsKey.Arn, secrets.Arn, jobRole.Name, executeRole.Name, snsAlert.Arn).ApplyT(
			func(args []interface{}) *pulumi.Output {
				configBucketArn := args[0].(string)
				credsBucketArn := args[1].(string)
				kmsKeyArn := args[2].(string)
				secretsArn := args[3].(string)
				jobRoleName := args[4].(string)
				executeRoleName := args[5].(string)
				snsAlertArn := args[6].(string)

				//
				// Job role access
				//

				jobsPolicyData, _ := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
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
								snsAlertArn,
							},
						},
					},
				}, nil)
				jobPolicy, err := iam.NewPolicy(ctx, "job-resource-policy", &iam.PolicyArgs{
					Path:        pulumi.String("/"),
					Name:        pulumi.String(fmt.Sprintf("%s-%s-job", ctx.Project(), ctx.Stack())),
					Description: pulumi.String("S3 and KMS Access"),
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
				executePolicy, err := iam.NewPolicy(ctx, "execute-resource-policy", &iam.PolicyArgs{
					Path:        pulumi.String("/"),
					Name:        pulumi.String(fmt.Sprintf("%s-%s-execute", ctx.Project(), ctx.Stack())),
					Description: pulumi.String("Secrets Manager and KMS Access"),
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

		//
		// Create pipeline policy and user for pushing docker image
		//

		pulumi.All(dockerRepo.Arn).ApplyT(
			func(args []interface{}) *pulumi.Output {
				repoArn := args[0].(string)
				userPolicyData, _ := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
					Statements: []iam.GetPolicyDocumentStatement{
						{
							Actions: []string{
								"ecr:CompleteLayerUpload",
								"ecr:UploadLayerPart",
								"ecr:InitiateLayerUpload",
								"ecr:BatchCheckLayerAvailability",
								"ecr:PutImage",
							},
							Resources: []string{
								repoArn,
							},
						},
						{
							Actions: []string{
								"ecr:GetAuthorizationToken",
							},
							Resources: []string{
								"*",
							},
						},
					},
				}, nil)
				user, err := iam.NewUser(ctx, "repo-pusher", &iam.UserArgs{
					Path: pulumi.String("/"),
					Name: pulumi.String(fmt.Sprintf("%s-%s-repo", ctx.Project(), ctx.Stack())),
				})
				if err != nil {
					log.Fatal(err)
				}
				_, err = iam.NewUserPolicy(ctx, "repo-pusher-policy", &iam.UserPolicyArgs{
					User:   user.Name,
					Policy: pulumi.String(userPolicyData.Json),
				})
				if err != nil {
					log.Fatal(err)
				}
				return nil
			})

		ctx.Export("Docker Repo Url", dockerRepo.RepositoryUrl)
		ctx.Export("Config Bucket", configBucket.Bucket)
		ctx.Export("Creds Bucket", credsBucket.Bucket)
		return nil
	})
}
