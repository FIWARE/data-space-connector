const dns_broker = `http://${process.env.DNS_CONTEXT_BROKER}/ngsi-ld/v1`;
const URL_SMART_DATA_MODEL = process.env.URL_SMART_DATA_MODEL;
const IOTDOMAINNAME = process.env.IOTDOMAINNAME;
const LAKERULENAME = process.env.LAKERULENAME;
const SUBNAME = process.env.SUBNAME;
const axios = require("axios");

exports.handler = async (event, context) => {
  console.log(IOTDOMAINNAME);
  console.log(LAKERULENAME);
  console.log(SUBNAME);

  let request_type = event['RequestType'].toLowerCase()
  if (request_type=='create' || request_type == 'update') {
    try {
      let allsub = {
        id: `urn:ngsi-ld:Subscription:${SUBNAME}`,
        description: "GARNET DATALAKE SUB - DO NOT DELETE",
        type: "Subscription",
        entities: [{ type: "*" }],
        notification: {
          format: "concise",
          endpoint: {
            uri: `https://${IOTDOMAINNAME}/topics/$aws/rules/${LAKERULENAME}?qos=1`,
            accept: "application/json",
          },
        },
      }
      const headers = {
        'Content-Type': 'application/json',
        'Link': `<${URL_SMART_DATA_MODEL}>; rel="http://www.w3.org/ns/json-ld#context"; type="application/ld+json"`
        }
      let addsub = await axios.post(`${dns_broker}/subscriptions`, allsub, {
        headers: headers,
      })
    } catch (e) {
      log_error(event, context, e.message, e);
    }
  }
};

const log_error = (event, context, message, error) => {
  console.error(
    JSON.stringify({
      message: message,
      event: event,
      error: error,
      context: context,
    })
  );
};
