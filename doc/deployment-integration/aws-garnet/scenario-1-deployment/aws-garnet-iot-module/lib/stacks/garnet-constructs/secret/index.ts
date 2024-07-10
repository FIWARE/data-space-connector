import { NestedStack, NestedStackProps } from "aws-cdk-lib"
import { Secret } from "aws-cdk-lib/aws-secretsmanager"
import { Construct } from "constructs"
import { Parameters } from "../../../../parameters"


export interface GarnetSecretProps {
}

export class GarnetSecret extends Construct {
    public readonly secret: Secret

    constructor(scope: Construct, id: string, props: GarnetSecretProps) {
        super(scope, id)
    
        this.secret = new Secret(this, 'Secret', {
            secretName:`garnet/secret/${Parameters.garnet_broker.toLowerCase()}/${this.node.addr.slice(-4)}`,
            generateSecretString: {
                secretStringTemplate: JSON.stringify({
                  username: 'garnetadmin',
                }),
                excludePunctuation: true,
                excludeCharacters: "/¥'%:;{}",
                includeSpace: false,
                generateStringKey: 'password'
              }
        })
    
    }

}