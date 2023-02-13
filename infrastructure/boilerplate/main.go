package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		//
		// Networking resources
		//

		vpc, err := ec2.NewVpc(ctx, "main", &ec2.VpcArgs{
			CidrBlock: pulumi.String("10.0.0.0/16"),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%v-%v", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
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
				"Name": pulumi.String(fmt.Sprintf("%v-%v", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		internetGateway, err := ec2.NewInternetGateway(ctx, "internetGateway", &ec2.InternetGatewayArgs{
			VpcId: vpc.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%v-%v", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		subnetPublic, err := ec2.NewSubnet(ctx, "public", &ec2.SubnetArgs{
			VpcId:     vpc.ID(),
			CidrBlock: pulumi.String("10.0.0.0/20"),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%v-%v-public", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		subnetPrivate, err := ec2.NewSubnet(ctx, "private", &ec2.SubnetArgs{
			VpcId:     vpc.ID(),
			CidrBlock: pulumi.String("10.0.128.0/20"),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%v-%v-private", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		elasticIp, err := ec2.NewEip(ctx, "elasticip", &ec2.EipArgs{
			Vpc: pulumi.Bool(true),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%v-%v", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		natGateway, err := ec2.NewNatGateway(ctx, "nat-gateway", &ec2.NatGatewayArgs{
			AllocationId: elasticIp.ID(),
			SubnetId:     subnetPublic.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%v-%v", ctx.Project(), ctx.Stack())),
			},
		}, pulumi.DependsOn([]pulumi.Resource{
			internetGateway,
		}))
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
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
				"Name": pulumi.String(fmt.Sprintf("%v-%v-public", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
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
				"Name": pulumi.String(fmt.Sprintf("%v-%v-private", ctx.Project(), ctx.Stack())),
			},
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		_, err = ec2.NewRouteTableAssociation(ctx, "rta-Public", &ec2.RouteTableAssociationArgs{
			SubnetId:     subnetPublic.ID(),
			RouteTableId: routePublic.ID(),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}
		_, err = ec2.NewRouteTableAssociation(ctx, "rta-Private", &ec2.RouteTableAssociationArgs{
			SubnetId:     subnetPrivate.ID(),
			RouteTableId: routePrivate.ID(),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}

		//
		// ECR Repository
		//

		dockerRepo, err := ecr.NewRepository(ctx, "docker-repo", &ecr.RepositoryArgs{
			ImageScanningConfiguration: &ecr.RepositoryImageScanningConfigurationArgs{
				ScanOnPush: pulumi.Bool(false),
			},
			ImageTagMutability: pulumi.String("MUTABLE"),
			Name:               pulumi.String(fmt.Sprintf("%v-%v", ctx.Project(), ctx.Stack())),
		})
		if err != nil {
			ctx.Log.Error(err.Error(), nil)
		}

		//
		// Create pipeline policy and user for pushing docker images
		//

		pulumi.All(dockerRepo.Arn).ApplyT(
			func(args []interface{}) *pulumi.Output {
				repoArn := args[0].(string)
				userPolicyData, err := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
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
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}
				user, err := iam.NewUser(ctx, "repo-pusher", &iam.UserArgs{
					Path: pulumi.String("/"),
					Name: pulumi.String(fmt.Sprintf("%s-%s-repo", ctx.Project(), ctx.Stack())),
				})
				ctx.Export("docker repo iam user pusher", user.Name)
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}
				_, err = iam.NewUserPolicy(ctx, "repo-pusher-policy", &iam.UserPolicyArgs{
					User:   user.Name,
					Policy: pulumi.String(userPolicyData.Json),
				})
				if err != nil {
					ctx.Log.Error(err.Error(), nil)
				}
				return nil
			})

		//
		// Export the name of the bucket
		//

		ctx.Export("docker-repo-url", dockerRepo.RepositoryUrl)
		ctx.Export("security-group", securityGroup.ID())
		ctx.Export("vpc-id", vpc.ID())
		ctx.Export("private-subnet", subnetPrivate.ID())
		ctx.Export("public-subnet", subnetPublic.ID())
		return nil
	})
}
