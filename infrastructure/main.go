package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/batch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		vpc, err := ec2.NewVpc(ctx, "arkstorm", &ec2.VpcArgs{
			CidrBlock: pulumi.String("10.0.0.0/16"),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("arkstorm"),
			},
		})
		if err != nil {
			return err
		}

		securityGroup, err := ec2.NewDefaultSecurityGroup(ctx, "arkstorm-vpc-default-securitygroup", &ec2.DefaultSecurityGroupArgs{
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
				"Name": pulumi.String("arkstorm"),
			},
		})

		gateway, err := ec2.NewInternetGateway(ctx, "gw", &ec2.InternetGatewayArgs{
			VpcId: vpc.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("arkstorm"),
			},
		})
		if err != nil {
			return err
		}

		subnetPublic, err := ec2.NewSubnet(ctx, "public-subnet", &ec2.SubnetArgs{
			VpcId:     vpc.ID(),
			CidrBlock: pulumi.String("10.0.0.0/20"),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("arkstorm-public"),
			},
		})
		if err != nil {
			return err
		}

		subnetPrivate, err := ec2.NewSubnet(ctx, "private-subnet", &ec2.SubnetArgs{
			VpcId:     vpc.ID(),
			CidrBlock: pulumi.String("10.0.128.0/20"),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("arkstorm-private"),
			},
		})
		if err != nil {
			return err
		}

		ip, err := ec2.NewEip(ctx, "one", &ec2.EipArgs{
			Vpc: pulumi.Bool(true),
		})

		nat, err := ec2.NewNatGateway(ctx, "example", &ec2.NatGatewayArgs{
			AllocationId: ip.ID(),
			SubnetId:     subnetPublic.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("gw NAT"),
			},
		}, pulumi.DependsOn([]pulumi.Resource{
			gateway,
		}))

		routePublic, err := ec2.NewRouteTable(ctx, "route-public", &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock: pulumi.String("0.0.0.0/0"),
					GatewayId: gateway.ID(),
				},
			},
			Tags: pulumi.StringMap{
				"Name": pulumi.String("arkstorm-public"),
			},
		})

		routePrivate, err := ec2.NewRouteTable(ctx, "route-private", &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock: pulumi.String("0.0.0.0/0"),
					GatewayId: nat.ID(),
				},
			},
			Tags: pulumi.StringMap{
				"Name": pulumi.String("arkstorm-Private"),
			},
		})

		rtaPublic, err := ec2.NewRouteTableAssociation(ctx, "rta-Public", &ec2.RouteTableAssociationArgs{
			SubnetId:     subnetPublic.ID(),
			RouteTableId: routePublic.ID(),
		})

		rtaPrivate, err := ec2.NewRouteTableAssociation(ctx, "rta-Private", &ec2.RouteTableAssociationArgs{
			SubnetId:     subnetPrivate.ID(),
			RouteTableId: routePrivate.ID(),
		})

		repo, err := ecr.NewRepository(ctx, "arkstormrepo", &ecr.RepositoryArgs{
			ImageScanningConfiguration: &ecr.RepositoryImageScanningConfigurationArgs{
				ScanOnPush: pulumi.Bool(false),
			},
			ImageTagMutability: pulumi.String("MUTABLE"),
			Name:               pulumi.String("arkstormrepo"),
		})

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

		//
		// Execute Role.
		//

		executeRole, err := iam.NewRole(ctx, "arkstorm-execute", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(ecsAssumeRole),
			Name:             pulumi.String("arkstorm-execute"),
		})
		_, err = iam.NewRolePolicyAttachment(ctx, "arkstorm-execute-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      executeRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		})

		//
		// Job Role
		//

		jobRole, err := iam.NewRole(ctx, "arkstorm-job", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(ecsAssumeRole),
			Name:             pulumi.String("arkstorm-job"),
		})
		_, err = iam.NewRolePolicyAttachment(ctx, "arkstorm-batch-job-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      jobRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
		})
		_, err = iam.NewRolePolicyAttachment(ctx, "arkstorm-batch-job-s3", &iam.RolePolicyAttachmentArgs{
			Role:      jobRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess"),
		})

		//
		// Compute Role
		//

		computeRole, err := iam.NewServiceLinkedRole(ctx, "arkstorm-compute", &iam.ServiceLinkedRoleArgs{
			AwsServiceName: pulumi.String("batch.amazonaws.com"),
		})

		//
		// Compute environment.
		//

		computeEnv, err := batch.NewComputeEnvironment(ctx, "arkstorm-compute-deleteme", &batch.ComputeEnvironmentArgs{
			ComputeEnvironmentName: pulumi.String("arkstorm-compute-deleteme"),
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
			return err
		}

		jobQueue, err := batch.NewJobQueue(ctx, "testQueue", &batch.JobQueueArgs{
			State:    pulumi.String("ENABLED"),
			Priority: pulumi.Int(1),
			ComputeEnvironments: pulumi.StringArray{
				computeEnv.Arn,
			},
		})

		jobDef := pulumi.All(jobRole.Arn, executeRole.Arn).ApplyT(
			func(args []interface{}) *batch.JobDefinition {
				jobRoleArn := args[0].(string)
				executeRoleArn := args[1].(string)

				jobDefContainerProperties, err := json.Marshal(map[string]interface{}{
					"image":            "602525097839.dkr.ecr.us-west-2.amazonaws.com/arkstormrepo:latest",
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
				})
				if err != nil {
					log.Fatal(err)
				}

				definition, err := batch.NewJobDefinition(ctx, "stormdef", &batch.JobDefinitionArgs{
					PlatformCapabilities: pulumi.StringArray{
						pulumi.String("FARGATE"),
					},
					ContainerProperties: pulumi.String(jobDefContainerProperties),
					Type:                pulumi.String("container"),
					Name:                pulumi.String("arkstormdef"),
				})

				return definition
			})

		fmt.Println(gateway)
		fmt.Println(securityGroup)
		fmt.Println(subnetPublic)
		fmt.Println(subnetPrivate)
		fmt.Println(nat)
		fmt.Println(ip)
		fmt.Println(routePublic)
		fmt.Println(routePrivate)
		fmt.Println(rtaPublic)
		fmt.Println(rtaPrivate)
		fmt.Println(repo)
		fmt.Println(executeRole)
		fmt.Println(jobRole)
		fmt.Println(computeRole)
		fmt.Println(computeEnv)
		fmt.Println(jobQueue)
		fmt.Println(jobDef)

		return nil
	})
}
