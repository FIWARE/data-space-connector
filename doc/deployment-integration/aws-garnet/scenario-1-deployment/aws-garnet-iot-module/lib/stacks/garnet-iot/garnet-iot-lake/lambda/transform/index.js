exports.handler = async (event, context) => {

    const output = event.records.map((record) => ({
        /* This transformation is the "identity" transformation, the data is left intact */
        recordId: record.recordId,
        result: 'Ok',
        data: Buffer.from(JSON.stringify(transform(JSON.parse(Buffer.from(record.data, 'base64').toString('utf8')))), 'utf-8').toString('base64')
    }))
    return { records: output }
}

const recursive_concise = (key, value) => {

    if( typeof value == 'object' && !Array.isArray(value)){
        if(['Property', 'Relationship', 'GeoProperty'].includes(value["type"])) {
            delete value["type"] 
        }
        for (let [skey, svalue] of Object.entries(value)){
                    recursive_concise(skey,svalue)
        }
    }
}

const transform = (payload) => {
    if(!payload?.type) return 
    if(payload?.type?.includes('#')){
        payload.type = `${payload.type.split('#').slice(-1)}`
    }
    if(payload?.type?.includes('/')){
        payload.type = `${payload.type.split('/').slice(-1)}`
    }
    for (let [key, value] of Object.entries(payload)) {
        if(!["type", "id", "@context"].includes(key)) {
            if( typeof value == "object" && !Array.isArray(value)){
                recursive_concise(key, value)
            } else {
                payload[key] = {
                    "value": value
                }
            }
        } 
    }
    console.log(payload)
    return payload
}
