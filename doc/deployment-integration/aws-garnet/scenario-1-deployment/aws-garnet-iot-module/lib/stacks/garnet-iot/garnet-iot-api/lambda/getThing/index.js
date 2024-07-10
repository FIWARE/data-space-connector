const iot_region = process.env.AWSIOTREGION 
const shadow_prefix = process.env.SHADOW_PREFIX

const { IoTDataPlaneClient, ListNamedShadowsForThingCommand, GetThingShadowCommand  } = require("@aws-sdk/client-iot-data-plane")
const { IoTClient, DeleteThingCommand} = require("@aws-sdk/client-iot")
const iot = new IoTClient({region: iot_region})
const iotdata = new IoTDataPlaneClient({region: iot_region})
const { toUtf8 } = require('@aws-sdk/util-utf8-browser')

exports.handler = async (event) => {

    try { 
        const {pathParameters: {thingName}, headers, queryStringParameters} = event

        let shadows = null
        if(!thingName) {
            throw new Error('thingName is required')
        }
        if(queryStringParameters && 'shadows' in queryStringParameters) {
            shadows = queryStringParameters['shadows'].split(',').map((shadow) => `${shadow_prefix}-${shadow.trim()}`)

        }
        console.log({queryStringParameters})

        let {results} = await iotdata.send(
            new ListNamedShadowsForThingCommand({ thingName })
        )

        if(!results.includes(`${shadow_prefix}-Thing`)) {
            return {
                statusCode: 404, 
                body: JSON.stringify({
                    message: `${thingName} is not registered in the Garnet IoT registry.`
                })
            }
        }
        console.log({results})
        if(shadows){
            results = results.filter(result => shadows.includes(result))
        }

        console.log({shadows})
        console.log({results})

        let result = {
            thingName, 
            entities: []
        }

        for await (let shadowName of results) {
          
            let {payload} = await iotdata.send(
                new GetThingShadowCommand({ thingName, shadowName})
            )
            
            result.entities.push(
                JSON.parse( toUtf8(payload) ).state.reported
            )
        } 

        const response = {
            statusCode: 200,
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify(result),
        }
        console.log(response)

        return response

    } catch(e){
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