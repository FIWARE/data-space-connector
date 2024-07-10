const iot_region = process.env.AWSIOTREGION 
const shadow_prefix = process.env.SHADOW_PREFIX
const dns_broker = `http://${process.env.DNS_CONTEXT_BROKER}/ngsi-ld/v1`

const axios = require('axios')
const { IoTDataPlaneClient, DeleteThingShadowCommand,ListNamedShadowsForThingCommand } = require("@aws-sdk/client-iot-data-plane")
const { IoTClient, DeleteThingCommand} = require("@aws-sdk/client-iot")
const iot = new IoTClient({region: iot_region})
const iotdata = new IoTDataPlaneClient({region: iot_region})

exports.handler = async (event) => {

    try { 
        const {pathParameters: {thingName}, headers, queryStringParameters} = event

        console.log(queryStringParameters?.['recursive'])
        
        if(!thingName) {
            throw new Error('thingName is required')
        }

        if (queryStringParameters?.['recursive'] == 'true'){
            try {
                const {results} = await iotdata.send(
                    new ListNamedShadowsForThingCommand({thingName})
                )
                for await (let shadowName of results){
                    if(shadowName.startsWith(shadow_prefix)){
                        try {
                            await iotdata.send(
                                new DeleteThingShadowCommand({ thingName, shadowName })
                            )
                            let delete_entity = await axios.delete(`${dns_broker}/entities/urn:ngsi-ld:${shadowName.split(shadow_prefix)[1].split('-')[1]}:${thingName}`)       
                        } catch (e) {
                            console.log(e.message)
                        }
                        
                    }
                }
            } catch(e){
                console.log(e.message)
            }

        } else {
            try {
                await iotdata.send(
                    new DeleteThingShadowCommand({
                        thingName, shadowName: `${shadow_prefix}-Thing`   
                    })
                )
                let delete_entity = await axios.delete(`${dns_broker}/entities/urn:ngsi-ld:Thing:${thingName}`)
            } catch (e) {
                console.log(e.message)
            }
        }

        await iot.send(
            new DeleteThingCommand({thingName})
        )
        let add = ''
        if(queryStringParameters?.['recursive'] == 'true'){
            add = 'and all its associated entities'
        }

        let msg = `Successfully deleted the thing ${thingName} ${add}`

        const response = {
            statusCode: 200,
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({message: msg}),
        }
        return response
    }
    catch(e){
        const response = {
            statusCode: 500,
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({message: e.message}),
        }
        console.log(e)
        return response
    }
}