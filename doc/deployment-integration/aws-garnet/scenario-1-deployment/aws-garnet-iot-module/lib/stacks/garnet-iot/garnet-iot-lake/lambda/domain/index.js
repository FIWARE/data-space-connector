const { IoTClient, CreateDomainConfigurationCommand, DescribeDomainConfigurationCommand, UpdateDomainConfigurationCommand } = require("@aws-sdk/client-iot")
const iot = new IoTClient({})
const DOMAIN_NAME = process.env.DOMAIN_NAME 
const AUTHORIZER_NAME = process.env.AUTHORIZER_NAME

exports.handler = async (event) => {
    console.log(event)
    let request_type = event['RequestType'].toLowerCase()
    if (request_type=='create' || request_type == 'update') {

        try {
            try {
                await iot.send(
                    new UpdateDomainConfigurationCommand({
                        domainConfigurationName: DOMAIN_NAME, 
                        domainConfigurationStatus: 'ENABLED', 
                        authorizerConfig: {
                            defaultAuthorizerName: `${AUTHORIZER_NAME}`,
                            allowAuthorizerOverride: false
                        }
                    })
                )
            } catch (e) {
                console.log(e.message)
                if(e.name == 'ResourceNotFoundException'){
            
                    await iot.send(
                        new CreateDomainConfigurationCommand({
                            domainConfigurationName: DOMAIN_NAME, 
                            domainConfigurationStatus: 'ENABLED', 
                            authorizerConfig: {
                                defaultAuthorizerName: `${AUTHORIZER_NAME}`,
                                allowAuthorizerOverride: false
                            }
                        })
                    )
    
                }
                
            }

            const {domainName, domainConfigurationArn, domainConfigurationName, domainConfigurationStatus} = await iot.send(
                new DescribeDomainConfigurationCommand({
                    domainConfigurationName: DOMAIN_NAME
                })
            )
          
            console.log({domainName})

            return {
                Data: {
                    domainName: domainName,
                }
            }
            
        } catch (e) {
            console.log(e)

            return null 
        }



    }
}