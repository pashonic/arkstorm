package main

import (
	"encoding/json"
	"fmt"

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
		// Batch execute Role.
		//

		batchExecuteRole, err := iam.NewRole(ctx, "arkstorm-batch-execute", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(ecsAssumeRole),
			Name:             pulumi.String("arkstorm-batch"),
		})
		_, err = iam.NewRolePolicyAttachment(ctx, "arkstorm-batch-execute-ecs-task", &iam.RolePolicyAttachmentArgs{
			Role:      batchExecuteRole.Name,
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
		// Compute environment.
		//
		/*
			_, err = batch.NewComputeEnvironment(ctx, "sampleComputeEnvironment", &batch.ComputeEnvironmentArgs{
				ComputeEnvironmentName: pulumi.String("sample"),
				ComputeResources: &batch.ComputeEnvironmentComputeResourcesArgs{
					InstanceRole: ecsInstanceRoleInstanceProfile.Arn,
					InstanceTypes: pulumi.StringArray{
						pulumi.String("c4.large"),
					},
					MaxVcpus: pulumi.Int(16),
					MinVcpus: pulumi.Int(0),
					SecurityGroupIds: pulumi.StringArray{
						sampleSecurityGroup.ID(),
					},
					Subnets: pulumi.StringArray{
						sampleSubnet.ID(),
					},
					Type: pulumi.String("ECS"),
				},
				ServiceRole: awsBatchServiceRoleRole.Arn,
				Type:        pulumi.String("MANAGED"),
			}, pulumi.DependsOn([]pulumi.Resource{
				awsBatchServiceRoleRolePolicyAttachment,
			}))*/

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
		fmt.Println(batchExecuteRole)
		fmt.Println(jobRole)

		return nil
	})
}
