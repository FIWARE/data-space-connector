import { Aws, Duration, Names } from "aws-cdk-lib";
import { SubnetType, Vpc } from "aws-cdk-lib/aws-ec2";
import {
  ManagedPolicy,
  PolicyStatement,
  Role,
  ServicePrincipal,
} from "aws-cdk-lib/aws-iam";
import { CfnTopicRule } from "aws-cdk-lib/aws-iot";
import { CfnDeliveryStream } from "aws-cdk-lib/aws-kinesisfirehose";
import {
  Code,
  LayerVersion,
  Runtime,
  Function,
  Architecture,
} from "aws-cdk-lib/aws-lambda";
import { SqsEventSource } from "aws-cdk-lib/aws-lambda-event-sources";
import { RetentionDays } from "aws-cdk-lib/aws-logs";
import { Bucket } from "aws-cdk-lib/aws-s3";
import { Topic } from "aws-cdk-lib/aws-sns";
import { SqsSubscription } from "aws-cdk-lib/aws-sns-subscriptions";
import { Queue } from "aws-cdk-lib/aws-sqs";
import {
  AwsCustomResource,
  AwsCustomResourcePolicy,
  PhysicalResourceId,
} from "aws-cdk-lib/custom-resources";
import { Construct } from "constructs";
import { Parameters } from "../../../../parameters";

export interface GarnetIotprops {
  dns_context_broker: string;
  vpc: Vpc;
  bucket_arn: string;
}

export class GarnetIot extends Construct {
  public readonly sqs_garnet_iot_arn: string;
  public readonly sns_garnet_iot: Topic;

  constructor(scope: Construct, id: string, props: GarnetIotprops) {
    super(scope, id);

    //CHECK PROPS
    if (!props.vpc) {
      throw new Error(
        "The property vpc is required to create an instance of GarnetIot Construct"
      );
    }
    if (!props.dns_context_broker) {
      throw new Error(
        "The property dns_context_broker is required to create an instance of GarnetIot Construct"
      );
    }
    if (!props.bucket_arn) {
      throw new Error(
        "The property bucket_arn is required to create an instance of GarnetIot Construct"
      );
    }

    // IoT DATALAKE BUCKET
    const bucket = Bucket.fromBucketArn(this, "IoTBucket", props.bucket_arn);

    // LAMBDA LAYER (SHARED LIBRARIES)
    const layer_lambda_path = `./lib/stacks/garnet-iot/layers`;
    const layer_lambda = new LayerVersion(this, "LayerLambda", {
      code: Code.fromAsset(layer_lambda_path),
      compatibleRuntimes: [Runtime.NODEJS_18_X],
    });

    // SQS ENTRY POINT
    const sqs_garnet_endpoint = new Queue(this, "SqsGarnetIot", {
      queueName: `garnet-iot-queue-${Aws.REGION}`,
    });
    this.sqs_garnet_iot_arn = sqs_garnet_endpoint.queueArn;

    // LAMBDA TO UPDATE DEVICE SHADOW
    const lambda_update_shadow_path = `${__dirname}/lambda/updateShadow`;
    const lambda_update_shadow = new Function(this, "LambdaUpdateShadow", {
      functionName: `garnet-iot-update-shadow-lambda-${Names.uniqueId(this)
        .slice(-8)
        .toLowerCase()}`,
      runtime: Runtime.NODEJS_18_X,
      code: Code.fromAsset(lambda_update_shadow_path),
      handler: "index.handler",
      timeout: Duration.seconds(15),
      logRetention: RetentionDays.THREE_MONTHS,
      architecture: Architecture.ARM_64,
      environment: {
        AWSIOTREGION: Aws.REGION,
        SHADOW_PREFIX: Parameters.garnet_iot.shadow_prefix
      },
    });

    // ADD PERMISSION FOR LAMBDA THAT UPDATES SHADOW TO ACCESS SQS ENTRY POINT
    lambda_update_shadow.addToRolePolicy(
      new PolicyStatement({
        actions: [
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes",
        ],
        resources: [`${sqs_garnet_endpoint.queueArn}`],
      })
    );

    // ADD PERMISSION TO ACCESS AWS IoT DEVICE SHADOW
    lambda_update_shadow.addToRolePolicy(
      new PolicyStatement({
        actions: ["iot:UpdateThingShadow"],
        resources: [
          `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:thing/*/${Parameters.garnet_iot.shadow_prefix}-*`,
        ],
      })
    );

    // ADD THE SQS ENTRY POINT AS EVENT SOURCE FOR LAMBDA
    lambda_update_shadow.addEventSource(
      new SqsEventSource(sqs_garnet_endpoint, { batchSize: 10 })
    );

    // SQS TO LAMBDA CONTEXT BROKER
    const sqs_to_context_broker = new Queue(this, "SqsToLambdaContextBroker", {
      queueName: `garnet-iot-contextbroker-queue-${Aws.REGION}`
    });

    // ROLE THAT GRANTS ACCESS TO FIREHOSE TO READ/WRITE BUCKET
    const role_firehose = new Role(this, "FirehoseRole", {
      assumedBy: new ServicePrincipal("firehose.amazonaws.com"),
    });
    bucket.grantReadWrite(role_firehose);

    // KINESIS FIREHOSE DELIVERY STREAM
    const kinesis_firehose = new CfnDeliveryStream(
      this,
      "KinesisFirehoseDeliveryGarnetIotDataLake",
      {
        deliveryStreamName: `garnet-iot-firehose-stream-${Names.uniqueId(this).slice(-8).toLowerCase()}`,
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
    );

    // ROLE THAT GRANT ACCESS TO IOT RULE TO ACTIONS
    const iot_rule_actions_role = new Role(this, "RoleGarnetIotRuleIngestion", {
      assumedBy: new ServicePrincipal("iot.amazonaws.com"),
    });
    iot_rule_actions_role.addToPolicy(
      new PolicyStatement({
        resources: [
          `${sqs_to_context_broker.queueArn}`,
          `${kinesis_firehose.attrArn}`,
        ],
        actions: [
          "sqs:SendMessage",
          "firehose:DescribeDeliveryStream",
          "firehose:ListDeliveryStreams",
          "firehose:ListTagsForDeliveryStream",
          "firehose:PutRecord",
          "firehose:PutRecordBatch",
        ],
      })
    );

    // IOT RULE THAT LISTENS TO CHANGES IN GARNET SHADOWS AND PUSH TO SQS
    const iot_rule = new CfnTopicRule(this, "IoTRuleShadows", {
      ruleName: `garnet_iot_rule_${Names.uniqueId(this).slice(-8).toLowerCase()}`,
      topicRulePayload: {
        awsIotSqlVersion: "2016-03-23",
        ruleDisabled: false,
        sql: `SELECT current.state.reported.* 
                        FROM '$aws/things/+/shadow/name/+/update/documents' 
                        WHERE startswith(topic(6), '${Parameters.garnet_iot.shadow_prefix}') 
                        AND NOT isUndefined(current.state.reported.type)`,
        actions: [
          {
            sqs: {
              queueUrl: sqs_to_context_broker.queueUrl,
              roleArn: iot_rule_actions_role.roleArn,
            },
          },
          {
            firehose: {
              deliveryStreamName: kinesis_firehose.ref,
              roleArn: iot_rule_actions_role.roleArn,
              separator: "\n",
            },
          },
        ],
      },
    })


    // IOT RULE THAT LISTENS TO SUBSCRIPTIONS AND PUSH TO FIREHOSE
    const iot_rule_sub = new CfnTopicRule(this, "IotRuleSub", {
        ruleName: `garnet_subscriptions_rule_${Names.uniqueId(this).slice(-8).toLowerCase()}`,
        topicRulePayload: {
          awsIotSqlVersion: "2016-03-23",
          ruleDisabled: false,
          sql: `SELECT * FROM 'garnet/subscriptions/+'`,
          actions: [
            {
              firehose: {
                deliveryStreamName: kinesis_firehose.ref,
                roleArn: iot_rule_actions_role.roleArn,
                separator: "\n",
              },
            },
          ],
        },
      })




    // LAMBDA THAT GETS MESSAGES FROM THE QUEUE AND UPDATES CONTEXT BROKER
    const lambda_to_context_broker_path = `${__dirname}/lambda/updateContextBroker`;
    const lambda_to_context_broker = new Function(
      this,
      "LambdaUpdateContextBroker",
      {
        functionName: `garnet-iot-update-broker-lambda-${Names.uniqueId(this)
          .slice(-8)
          .toLowerCase()}`,
        vpc: props.vpc,
        vpcSubnets: {
          subnetType: SubnetType.PRIVATE_WITH_EGRESS,
        },
        runtime: Runtime.NODEJS_18_X,
        code: Code.fromAsset(lambda_to_context_broker_path),
        handler: "index.handler",
        timeout: Duration.seconds(15),
        logRetention: RetentionDays.THREE_MONTHS,
        layers: [layer_lambda],
        architecture: Architecture.ARM_64,
        environment: {
          DNS_CONTEXT_BROKER: props.dns_context_broker,
          URL_SMART_DATA_MODEL: Parameters.garnet_iot.smart_data_model_url,
          AWSIOTREGION: Aws.REGION,
          SHADOW_PREFIX: Parameters.garnet_iot.shadow_prefix
        },
      }
    );

    lambda_to_context_broker.addToRolePolicy(
      new PolicyStatement({
        actions: [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents",
          "ec2:CreateNetworkInterface",
          "ec2:DescribeNetworkInterfaces",
          "ec2:DeleteNetworkInterface",
          "ec2:AssignPrivateIpAddresses",
          "ec2:UnassignPrivateIpAddresses",
        ],
        resources: ["*"],
      })
    );

    // ADD PERMISSION FOR LAMBDA TO ACCESS SQS
    lambda_to_context_broker.addToRolePolicy(
      new PolicyStatement({
        actions: [
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes",
        ],
        resources: [`${sqs_to_context_broker.queueArn}`],
      })
    );

    lambda_to_context_broker.addToRolePolicy(
      new PolicyStatement({
        actions: ["iot:UpdateThingShadow", "iot:GetThingShadow"],
        resources: [
          `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:thing/*/${Parameters.garnet_iot.shadow_prefix}-*`,
        ],
      })
    );

    lambda_to_context_broker.addEventSource(
      new SqsEventSource(sqs_to_context_broker, { batchSize: 10 })
    );
  }
}
