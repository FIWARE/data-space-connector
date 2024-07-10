import { Aws, CfnOutput, Duration, Names } from "aws-cdk-lib"
import { EndpointType, LambdaRestApi } from "aws-cdk-lib/aws-apigateway"
import { InterfaceVpcEndpoint, Peer, Port, SecurityGroup, Vpc } from "aws-cdk-lib/aws-ec2"
import { AnyPrincipal, Effect, Policy, PolicyDocument, PolicyStatement, Role, ServicePrincipal } from "aws-cdk-lib/aws-iam"
import { Architecture, Code, Function, Runtime } from "aws-cdk-lib/aws-lambda"
import { Construct } from "constructs"
import { garnet_constant } from "../../garnet-constructs/constants"
import { CfnTopicRule } from "aws-cdk-lib/aws-iot"
import { CfnDeliveryStream } from "aws-cdk-lib/aws-kinesisfirehose"
import { Bucket } from "aws-cdk-lib/aws-s3"

export interface GarnetPrivateSubProps {
    vpc: Vpc, 
    bucket_name: string
  }
  
  export class GarnetPrivateSub extends Construct {
  
    public readonly private_sub_endpoint: string
  
    constructor(scope: Construct, id: string, props: GarnetPrivateSubProps) {
      super(scope, id)

        // SECURITY GROUP
        const sg_garnet_vpc_endpoint = new SecurityGroup(this, 'PrivateSubSecurityGroup', {
            securityGroupName: `garnet-private-sub-endpoint-sg-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
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
        functionName: `garnet-private-sub-lambda-${Names.uniqueId(this).slice(-4).toLowerCase()}`, 
        description: 'Garnet Private Sub - Function for the private subscription',
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
            resources: [
                `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:topic/garnet/subscriptions/*`,
            ]
        }))

        // POLICY FOR API 
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
            policy: api_policy,
            description: "Garnet Private Endpoint for Subscriptions",
            deployOptions: {
              stageName: "privatesub"
            }
        })

        this.private_sub_endpoint = api_private_sub.url
        
        new CfnOutput(this, 'ApiEndpoint', {
        value: api_private_sub.url,
        description: 'Private API Endpoint for Subscriptions'
        })


         // KINESIS FIREHOSE TO DATALAKE BUCKET 

    // DATALAKE BUCKET
    const bucket = Bucket.fromBucketName(this, "GarnetBucket", props.bucket_name)


    // ROLE THAT GRANTS ACCESS TO FIREHOSE TO READ/WRITE BUCKET
    const role_firehose = new Role(this, "FirehoseRole", {
      assumedBy: new ServicePrincipal("firehose.amazonaws.com"),
    });
    bucket.grantReadWrite(role_firehose)
      
    // KINESIS FIREHOSE DELIVERY STREAM
    const kinesis_firehose = new CfnDeliveryStream( this, "GarnetFirehose", {
        deliveryStreamName: `garnet-sub-firehose-stream-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
        deliveryStreamType: "DirectPut",
        extendedS3DestinationConfiguration: {
          bucketArn: bucket.bucketArn,
          roleArn: role_firehose.roleArn,
          bufferingHints: {
            intervalInSeconds: 60,
            sizeInMBs: 64,
          },
          processingConfiguration: {
            enabled: true,
            processors: [
              {
                type: "MetadataExtraction",
                parameters: [
                  {
                    parameterName: "MetadataExtractionQuery",
                    parameterValue: "{type:.type}",
                  },
                  {
                    parameterName: "JsonParsingEngine",
                    parameterValue: "JQ-1.6",
                  },
                ],
              },
            ],
          },
          dynamicPartitioningConfiguration: {
            enabled: true,
          },
          prefix: `type=!{partitionKeyFromQuery:type}/dt=!{timestamp:yyyy}-!{timestamp:MM}-!{timestamp:dd}-!{timestamp:HH}/`,
          errorOutputPrefix: `type=!{firehose:error-output-type}/dt=!{timestamp:yyy}-!{timestamp:MM}-!{timestamp:dd}-!{timestamp:HH}/`,
        },
      }
    )

        // IOT RULE THAT LISTENS TO SUBSCRIPTIONS AND PUSH TO FIREHOSE
        const iot_rule_sub_name = `garnet_subscriptions_rule_${Names.uniqueId(this).slice(-4).toLowerCase()}`
  
        const iot_rule_sub_role = new Role(this, "RoleGarnetIotRuleIngestion", {
          assumedBy: new ServicePrincipal("iot.amazonaws.com"),
        })

        const iot_rule_sub_policy = new Policy(this, 'PolicyGarnetIotRuleIngestion', {
          statements: [
            new PolicyStatement({
              resources: [ `${kinesis_firehose.attrArn}` ],
              actions: [
                "firehose:DescribeDeliveryStream",
                "firehose:ListDeliveryStreams",
                "firehose:ListTagsForDeliveryStream",
                "firehose:PutRecord",
                "firehose:PutRecordBatch",
              ]
            })
          ]
        })

        iot_rule_sub_role.attachInlinePolicy(iot_rule_sub_policy)

        const iot_rule_sub = new CfnTopicRule(this, "IotRuleSub", {
            ruleName: iot_rule_sub_name, 
            topicRulePayload: {
            awsIotSqlVersion: "2016-03-23",
            ruleDisabled: false,
            sql: `SELECT * FROM 'garnet/subscriptions/+'`,
            actions: [
                {
                firehose: {
                    deliveryStreamName: kinesis_firehose.ref,
                    roleArn: iot_rule_sub_role.roleArn,
                    separator: "\n",
                },
                },
            ],
            },
        })

    }
  }