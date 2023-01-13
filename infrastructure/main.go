package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		vpc, err := ec2.NewVpc(ctx, "arkstorm", &ec2.VpcArgs{
			CidrBlock: pulumi.String("10.0.0.0/16"),
		})
		if err != nil {
			return err
		}

		securityGroup, err := ec2.NewDefaultSecurityGroup(ctx, "arkstorm-vpc-default", &ec2.DefaultSecurityGroupArgs{
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
		})

		fmt.Println(securityGroup)

		return nil
	})
}
