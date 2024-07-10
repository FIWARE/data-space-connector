import { Aws, CfnOutput, Duration, Names } from "aws-cdk-lib"
import { EndpointType, LambdaRestApi } from "aws-cdk-lib/aws-apigateway"
import { InterfaceVpcEndpoint, Peer, Port, SecurityGroup, Vpc } from "aws-cdk-lib/aws-ec2"
import { AnyPrincipal, Effect, PolicyDocument, PolicyStatement } from "aws-cdk-lib/aws-iam"
import { Architecture, Code, Function, Runtime } from "aws-cdk-lib/aws-lambda"
import { Construct } from "constructs"

export interface GarnetPrivateSubProps {
    vpc: Vpc
  }
  
  export class GarnetPrivateSub extends Construct {
  
    public readonly private_sub_endpoint: string
  
    constructor(scope: Construct, id: string, props: GarnetPrivateSubProps) {
      super(scope, id)

        // SECURITY GROUP
        const sg_garnet_vpc_endpoint = new SecurityGroup(this, 'PrivateSubSecurityGroup', {
            securityGroupName: `garnet-private-sub-endpoint-sg-${Names.uniqueId(this).slice(-8).toLowerCase()}`,
            vpc: props.vpc,
            allowAllOutbound: true
        })
        sg_garnet_vpc_endpoint.addIngressRule(Peer.anyIpv4(), Port.tcp(443))

        // VPC ENDPOINT  
        const vpc_endpoint = new InterfaceVpcEndpoint(this, 'GarnetPrivateSubEndpoint', {
            vpc: props.vpc,
            service: {
            name: `com.amazonaws.${Aws.REGION}.execute-api`,
            port: 443
            },
            privateDnsEnabled: true,
            securityGroups: [sg_garnet_vpc_endpoint]
        })

        // LAMBDA 
        const lambda_garnet_private_sub_path = `${__dirname}/lambda/garnetSub`
        const lambda_garnet_private_sub = new Function(this, 'GarnetSubFunction', {
        functionName: `garnet-private-sub-lambda-${Names.uniqueId(this).slice(-8).toLowerCase()}`, 
            runtime: Runtime.NODEJS_18_X,
            code: Code.fromAsset(lambda_garnet_private_sub_path),
            handler: 'index.handler',
            timeout: Duration.seconds(50),
            architecture: Architecture.ARM_64,
            environment: {
            AWSIOTREGION: Aws.REGION
            }
        })

        lambda_garnet_private_sub.addToRolePolicy(new PolicyStatement({
            actions: ["iot:Publish"],
            resources: [`arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:topic/garnet/subscriptions/*`]
        }))

        // POLICY 
        const api_policy = new PolicyDocument({
            statements: [
            new PolicyStatement({
                principals: [new AnyPrincipal],
                actions: ['execute-api:Invoke'],
                resources: ['execute-api:/*'],
                effect: Effect.DENY,
                conditions: {
                StringNotEquals: {
                    "aws:SourceVpce": vpc_endpoint.vpcEndpointId
                }
                }
            }),
            new PolicyStatement({
                principals: [new AnyPrincipal],
                actions: ['execute-api:Invoke'],
                resources: ['execute-api:/*'],
                effect: Effect.ALLOW
            })
            ]
        })


        const api_private_sub = new LambdaRestApi(this, 'ApiPrivateSub', {
            restApiName:'garnet-private-sub-endpoint-api',
            endpointTypes: [EndpointType.PRIVATE], 
            handler: lambda_garnet_private_sub,
            policy: api_policy
          })

          this.private_sub_endpoint = api_private_sub.url
          
          new CfnOutput(this, 'ApiEndpoint', {
            value: api_private_sub.url,
            description: 'Private API Endpoint for Subscriptions'
          })



    }
  }