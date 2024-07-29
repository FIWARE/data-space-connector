import { Aws, Duration, Names } from "aws-cdk-lib";
import { SubnetType, Vpc } from "aws-cdk-lib/aws-ec2";
import { ManagedPolicy, PolicyStatement, Role, ServicePrincipal } from "aws-cdk-lib/aws-iam";
import { CfnTopicRule } from "aws-cdk-lib/aws-iot";
import { Code, LayerVersion, Runtime, Function,Architecture} from "aws-cdk-lib/aws-lambda";
import { SqsEventSource } from "aws-cdk-lib/aws-lambda-event-sources";
import { RetentionDays } from "aws-cdk-lib/aws-logs";
import { Topic } from "aws-cdk-lib/aws-sns";
import { Queue } from "aws-cdk-lib/aws-sqs";
import { Construct } from "constructs";
import { Parameters } from "../../../../parameters"
import { garnet_constant } from "../../garnet-constructs/constants";

export interface GarnetIotprops {
  dns_context_broker: string
  vpc: Vpc
}

export class GarnetIot extends Construct {
  public readonly sqs_garnet_iot_url: string;
  public readonly sns_garnet_iot: Topic;
  public readonly iot_rule_sub_name: string

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

    // LAMBDA LAYER (SHARED LIBRARIES)
    const layer_lambda_path = `./lib/layers`;
    const layer_lambda = new LayerVersion(this, "LayerLambda", {
      code: Code.fromAsset(layer_lambda_path),
      compatibleRuntimes: [Runtime.NODEJS_18_X],
    })

    // SQS ENTRY POINT
    const sqs_garnet_endpoint = new Queue(this, "SqsGarnetIot", {
      queueName: `garnet-iot-queue-${Aws.REGION}`,
      visibilityTimeout: Duration.seconds(55)
    })
    this.sqs_garnet_iot_url = sqs_garnet_endpoint.queueUrl;

    // LAMBDA TO UPDATE DEVICE SHADOW
    const lambda_update_shadow_path = `${__dirname}/lambda/updateShadow`;
    const lambda_update_shadow = new Function(this, "LambdaUpdateShadow", {
      functionName: `garnet-iot-update-shadow-lambda-${Names.uniqueId(this)
        .slice(-4)
        .toLowerCase()}`,
      description: 'Garnet IoT - Function that updates shadows',
      runtime: Runtime.NODEJS_18_X,
      code: Code.fromAsset(lambda_update_shadow_path),
      handler: "index.handler",
      timeout: Duration.seconds(50),
      logRetention: RetentionDays.THREE_MONTHS,
      architecture: Architecture.ARM_64,
      environment: {
        AWSIOTREGION: Aws.REGION,
        SHADOW_PREFIX: garnet_constant.shadow_prefix
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
          `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:thing/*/${garnet_constant.shadow_prefix}-*`,
        ],
      })
    );

    // ADD THE SQS ENTRY POINT AS EVENT SOURCE FOR LAMBDA
    lambda_update_shadow.addEventSource(
      new SqsEventSource(sqs_garnet_endpoint, { batchSize: 10 })
    );

    // SQS TO LAMBDA CONTEXT BROKER
    const sqs_to_context_broker = new Queue(this, "SqsToLambdaContextBroker", {
      queueName: `garnet-iot-contextbroker-queue-${Aws.REGION}`,
      visibilityTimeout: Duration.seconds(55)
    });


    // ROLE THAT GRANT ACCESS TO IOT RULE TO ACTIONS
    const iot_rule_actions_role = new Role(this, "RoleGarnetIotRuleIngestion", {
      assumedBy: new ServicePrincipal("iot.amazonaws.com"),
    });
    iot_rule_actions_role.addToPolicy(
      new PolicyStatement({
        resources: [
          `${sqs_to_context_broker.queueArn}`
        ],
        actions: [
          "sqs:SendMessage"
        ],
      })
    )

    // IOT RULE THAT LISTENS TO CHANGES IN GARNET SHADOWS AND PUSH TO SQS
    const iot_rule = new CfnTopicRule(this, "IoTRuleShadows", {
      ruleName: `garnet_iot_rule_${Names.uniqueId(this).slice(-4).toLowerCase()}`,
      topicRulePayload: {
        awsIotSqlVersion: "2016-03-23",
        ruleDisabled: false,
        sql: `SELECT current.state.reported.* 
                        FROM '$aws/things/+/shadow/name/+/update/documents' 
                        WHERE startswith(topic(6), '${garnet_constant.shadow_prefix}') 
                        AND NOT isUndefined(current.state.reported.type)`,
        actions: [
          {
            sqs: {
              queueUrl: sqs_to_context_broker.queueUrl,
              roleArn: iot_rule_actions_role.roleArn,
            },
          }
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
          .slice(-4)
          .toLowerCase()}`,
        description: 'Garnet IoT - Function that updates the context broker',
        vpc: props.vpc,
        vpcSubnets: {
          subnetType: SubnetType.PRIVATE_WITH_EGRESS,
        },
        runtime: Runtime.NODEJS_18_X,
        code: Code.fromAsset(lambda_to_context_broker_path),
        handler: "index.handler",
        timeout: Duration.seconds(50),
        logRetention: RetentionDays.THREE_MONTHS,
        layers: [layer_lambda],
        architecture: Architecture.ARM_64,
        environment: {
          DNS_CONTEXT_BROKER: props.dns_context_broker,
          URL_SMART_DATA_MODEL: Parameters.smart_data_model_url,
          AWSIOTREGION: Aws.REGION,
          SHADOW_PREFIX: garnet_constant.shadow_prefix
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
          `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:thing/*/${garnet_constant.shadow_prefix}-*`,
        ],
      })
    );

    lambda_to_context_broker.addEventSource(
      new SqsEventSource(sqs_to_context_broker, { batchSize: 10 })
    );
  }
}
