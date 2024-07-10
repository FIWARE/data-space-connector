const iot_region = process.env.AWSIOTREGION 
const shadow_prefix = process.env.SHADOW_PREFIX

const { IoTDataPlaneClient, UpdateThingShadowCommand } = require("@aws-sdk/client-iot-data-plane")
const iotdata = new IoTDataPlaneClient({region: iot_region})

exports.handler = async (event) => {

    try{
        try { JSON.parse(event.body) } catch(e){throw new Error('Invalid body property. The body must be a JSON object. Check the Content-Type in your header')}

        const {body, pathParameters: {thingName}} = event

        if(!thingName) {
            throw new Error('The thingName is required')
        }

        if(!body){
            throw new Error('An NGSI-LD entity is required in the body')
        }

        let entity = JSON.parse(body)
        console.log({entity}) 

        if(!entity.id || !entity.type) {
            throw new Error('id and type are required')
        }

        const entity_thingName = `${entity.id.split(':').slice(-1)}`

        if(entity_thingName != thingName){
            throw new Error(`The name ${entity_thingName} extracted from the id of the entity does not match ${thingName}`)
        }
        if(entity.type == 'Thing') {
            throw new Error(`Type Thing is not a valid type for this operation`)
        }
        if(!entity.id.startsWith(`urn:ngsi-ld:${entity.type}:`)){
            throw new Error(`Invalid id. The id must start with urn:ngsi-ld:${entity.type}: following with thingName`)
        }

        for (let [key, value] of Object.entries(entity)) {
            if(!['type', 'id', '@context'].includes(key)) {
        
                if( typeof value == 'object' && !Array.isArray(value)){
                recursive_concise(key, value)
                } else {
                    entity[key] = {
                        value: value
                    }
                }
                
            
            } 
        }

        if (entity['@context']) {
            console.log(`Removed the context:`)
            console.log(entity['@context'])
            delete entity['@context']
        }

        const shadow_payload = {
            state: {
                reported: entity
            }
        }

        let updateThingShadow = await iotdata.send(
            new UpdateThingShadowCommand({payload: JSON.stringify(shadow_payload ), thingName: thingName, shadowName: `${shadow_prefix}-${entity.type}`})
        )

        let msg = `The entity ${entity.id} successfully registered as ${shadow_prefix}-${entity.type} for ${thingName}`
        const response = {
            statusCode: 200,
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({message: msg}),
        }
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



const recursive_concise = (key, value) => {

    if( typeof value == 'object' && !Array.isArray(value)){
        if(['Property', 'Relationship', 'GeoProperty'].includes(value['type'])) {
            delete value['type'] 
        }
        for (let [skey, svalue] of Object.entries(value)){
                    recursive_concise(skey,svalue)
        }
    }
}
