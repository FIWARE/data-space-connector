const iot_region = process.env.AWSIOTREGION
const shadow_prefix = process.env.SHADOW_PREFIX
const dns_broker = `http://${process.env.DNS_CONTEXT_BROKER}/ngsi-ld/v1`
const URL_SMART_DATA_MODEL = process.env.URL_SMART_DATA_MODEL

const {IoTDataPlaneClient, UpdateThingShadowCommand, GetThingShadowCommand} = require('@aws-sdk/client-iot-data-plane')
const iotdata = new IoTDataPlaneClient({region: iot_region})

const axios = require('axios')




exports.handler = async (event, context) => {

    try {
        let entities = []

        for await (let msg of event.Records){
            let payload = JSON.parse(msg.body)
            const thingName = `${payload.id.split(':').slice(-1)}`
            if(!payload.id || !payload.type){
                throw new Error('Invalid entity: id or type is missing')
            }

            // CHECK IF LOCATION IS IN THE PAYLOAD. IF NOT GET IT FROM THING SHADOW. 
            if(!payload.location && payload.type != 'Thing') {
                
                try {
                    let {payload : device_shadow} = await iotdata.send(
                        new GetThingShadowCommand({
                            thingName: thingName,
                            shadowName: `${shadow_prefix}-Thing`
                        })
                    )

                    device_shadow = JSON.parse(
                        new TextDecoder('utf-8').decode(device_shadow)
                        )
                    payload.location = device_shadow.state.reported.location

                    if(payload.location){
                        const shadow_payload = {
                            state: {
                                reported: payload
                            }
                        }
                        let updateThingShadow = await iotdata.send(
                            new UpdateThingShadowCommand({
                                payload: JSON.stringify(shadow_payload), 
                                thingName: thingName, 
                                shadowName: `${shadow_prefix}-${payload.type}`
                            })
                        )
                    }



                } catch (e) {
                    console.log(e.message)
                }
            }
            if (payload.raw) delete payload.raw
            if (payload['@context']) {
                console.log(`Removed the context:`)
                console.log(payload['@context'])
                delete payload['@context']
            }
            entities.push(payload)
        }
        const headers = {
            'Content-Type': 'application/json',
            'Link': `<${URL_SMART_DATA_MODEL}>; rel="http://www.w3.org/ns/json-ld#context"; type="application/ld+json"`
            }
        try {
            let upsert = await axios.post(`${dns_broker}/entityOperations/upsert`, entities, {headers: headers}) 
        } catch (e) {
            log_error(event,context, e.message, e)  
        }
    } catch (e) {
        log_error(event,context, e.message, e)
    }
}


const log_error = (event, context, message, error) => {
    console.error(JSON.stringify({
        message: message,
        event: event,
        error: error, 
        context: context
    }))
}

