#!/usr/bin/env bash
#
# consent_grant.sh -- seed a *granted* consent record for a holder DID into the
# (POC) consent-manager, so the consent PIP constraint `consent:hasValidConsent`
# allows access for that DID.
#
# Topology: the consent-manager runs in the *trust-anchor* namespace (the central
# authority that knows all participants); the consent-facade and tm-forum-api stay
# at the *provider* and are reached cross-namespace via their .provider.svc FQDNs.
#
# The consent-manager derives its privacy notices/consents from the consent-facade
# (CONTRACT_SERVICE_BASE_URL), which serves participant self-descriptions at
# /participants/{tmforum-org-id} backed by the TMForum party API. So this seed:
#   1. find-or-creates real provider/consumer TMForum organizations (party API),
#   2. seeds the consent-manager participants with selfDescriptionURL pointing at
#      the consent-facade (http://consent-facade.provider.svc.cluster.local:8080/participants/{orgId}),
#   3. seeds a user, a provider UserIdentifier whose email is the holder DID
#      (DID-in-email, UF-3) and a granted Consent,
#   4. provisions one jwt-auth consumer per participant in the authority apisix
#      (keyed on the participant legalName = the token's participant_name claim),
#      so each participant's token validates at the facade (item 1),
#   5. verifies the exact two calls the consent-filter plugin makes (the receipt
#      build actually dereferences the participant SD through the consent-facade).
#
# Usage: ./doc/scripts/consent_grant.sh <holder-did> [namespace]
#   holder-did : the DID that appears as credentialSubject.id in the consumer's VC
#   namespace  : kubernetes namespace of the consent-manager (default: trust-anchor)
#
# Enforcement is the consent-filter APISIX plugin (the canonical, only path); it
# needs no extra wiring here - it authenticates with the participant client
# credentials (clientID 'consent-demo-provider' / clientSecret 'demo', created
# here) and derives the provider SD from /participants/me.
set -euo pipefail

DID="${1:?usage: consent_grant.sh <holder-did> [namespace]}"
NS="${2:-trust-anchor}"

POD=$(kubectl -n "$NS" get pods -o name 2>/dev/null | grep consent-manager | head -1 | cut -d/ -f2 || true)
[ -n "$POD" ] || { echo "consent-manager pod not found in namespace '$NS'" >&2; exit 1; }

# The seed runs *inside* the consent-manager pod (mongoose, the compiled models,
# MONGO_URI, JWT_SECRET_KEY, the consent key, and cross-namespace access to the
# provider's consent-facade + tm-forum-api). __DID__ is substituted below.
read -r -d '' JS <<'NODE' || true
const NM='/usr/src/app/node_modules/';
const mongoose=require(NM+'mongoose');
const jwt=require(NM+'jsonwebtoken');
const http=require('http');
const Participant=require('/usr/src/app/dist/src/models/Participant/Participant.model').default;
const User=require('/usr/src/app/dist/src/models/User/User.model').default;
const UserIdentifier=require('/usr/src/app/dist/src/models/UserIdentifier/UserIdentifier.model').default;
const Consent=require('/usr/src/app/dist/src/models/Consent/Consent.model').default;
const DID='__DID__';
const FACADE=process.env.CONTRACT_SERVICE_BASE_URL||'http://consent-facade.provider.svc.cluster.local:8080';
const TMF=process.env.TMF_API_URL||'http://tm-forum-api-svc.provider.svc.cluster.local:8080';
const PARTY=TMF+'/tmf-api/party/v4/organization';
const CK=process.env.X_VISIONSTRUST_CONSENT_KEY;
// authority apisix admin (same namespace as the consent-manager) + the shared
// signing key, used to provision one jwt-auth consumer per participant.
const APISIX_ADMIN=process.env.APISIX_ADMIN||'http://consent-authority-apisix-admin:9180';
const APISIX_ADMIN_KEY=process.env.APISIX_ADMIN_KEY||'admin';
const JWT_SECRET=process.env.JWT_SECRET_KEY;
function req(method,url,headers,body){const u=new URL(url);return new Promise((res,rej)=>{const data=body?JSON.stringify(body):null;const r=http.request({host:u.hostname,port:u.port||80,method,path:u.pathname+u.search,headers:{'content-type':'application/json',...(headers||{}),...(data?{'content-length':Buffer.byteLength(data)}:{})}},resp=>{let b='';resp.on('data',c=>b+=c);resp.on('end',()=>res({code:resp.statusCode,body:b}));});r.on('error',rej);if(data)r.write(data);r.end();});}
const cm=(method,p,headers,body)=>req(method,'http://localhost:3000'+p,headers,body);
// find (by name) or create a TMForum organization; returns its id
async function orgId(name,did,country){
  const list=await req('GET',PARTY+'?limit=1000');
  let orgs=[];try{orgs=JSON.parse(list.body);}catch(e){}
  const found=(Array.isArray(orgs)?orgs:[]).find(o=>o&&o.name===name);
  if(found) return found.id;
  const c=await req('POST',PARTY,{},{name,tradingName:name,isLegalEntity:true,organizationType:'company',
    contactMedium:[{characteristic:{country}}],partyCharacteristic:[{name:'did',value:did}]});
  if(c.code>=300) throw new Error('party API POST organization -> HTTP '+c.code+' '+c.body.slice(0,200));
  return JSON.parse(c.body).id;
}
// provision a jwt-auth consumer for a participant in the authority apisix, so its
// participant token (participant_name = legalName) validates at the facade. Keyed
// on the legalName, signed with the shared JWT_SECRET_KEY. Idempotent (PUT); this
// is the "provision when a participant registers" path the reconcile Job mirrors.
async function registerConsumer(legalName){
  const username=legalName.replace(/[^0-9a-zA-Z_]/g,'_');
  const r=await req('PUT',APISIX_ADMIN+'/apisix/admin/consumers',{'x-api-key':APISIX_ADMIN_KEY},{username,plugins:{'jwt-auth':{key:legalName,secret:JWT_SECRET,algorithm:'HS256'}}});
  if(r.code>=300) throw new Error('apisix consumer PUT for "'+legalName+'" -> HTTP '+r.code+' '+r.body.slice(0,150));
  return username;
}
(async()=>{
  await mongoose.connect(process.env.MONGO_URI);
  // 1) real TMForum orgs -> the participant self-description ids
  const provOrg=await orgId('Consent Demo Provider','did:web:mp-operations.org','DE');
  const consOrg=await orgId('Consent Demo Consumer','did:web:fancy-marketplace.biz','DE');
  const PROVIDER_SD=FACADE+'/participants/'+provOrg;
  const CONSUMER_SD=FACADE+'/participants/'+consOrg;
  // 2) consent-manager participants; selfDescriptionURL is served by the consent-facade
  const prov=await Participant.findOneAndUpdate({selfDescriptionURL:PROVIDER_SD},{$setOnInsert:{legalName:'M&P Operations Inc.',email:'provider@mp-operation.org',did:'did:web:mp-operations.org',selfDescriptionURL:PROVIDER_SD,clientID:'consent-demo-provider',clientSecret:'demo'}},{upsert:true,new:true,setDefaultsOnInsert:true});
  const cons=await Participant.findOneAndUpdate({selfDescriptionURL:CONSUMER_SD},{$setOnInsert:{legalName:'Fancy Marketplace Co.',email:'consumer@fancy-marketplace.biz',did:'did:web:fancy-marketplace.biz',selfDescriptionURL:CONSUMER_SD,clientID:'consent-demo-consumer',clientSecret:'demo'}},{upsert:true,new:true,setDefaultsOnInsert:true});
  // 2b) one jwt-auth consumer per participant in the authority apisix (item 1)
  const provConsumer=await registerConsumer(prov.legalName);
  const consConsumer=await registerConsumer(cons.legalName);
  const user=await User.findOneAndUpdate({email:DID},{$setOnInsert:{email:DID,firstName:'Consent',lastName:'Subject'}},{upsert:true,new:true,setDefaultsOnInsert:true});
  let ui=await UserIdentifier.findOne({attachedParticipant:prov._id,email:DID});
  if(!ui) ui=await UserIdentifier.create({attachedParticipant:prov._id,email:DID,identifier:DID,user:user._id});
  await User.updateOne({_id:user._id},{$addToSet:{identifiers:ui._id}});
  let consent=await Consent.findOne({providerUserIdentifier:ui._id,child:{$exists:false}});
  if(!consent) consent=await Consent.create({contract:FACADE+'/bilaterals/demo',providerUserIdentifier:ui._id,consented:true,dataProvider:prov._id,dataConsumer:cons._id,recipients:['did:web:fancy-marketplace.biz'],status:'granted',user:user._id,purposes:[{purpose:'demo-access'}],data:[{resource:'urn:ngsi-ld:PersonalProfile:alice'}],privacyNotice:'demo'});
  else { consent.status='granted'; consent.consented=true; await consent.save(); }
  const token=jwt.sign({sub:String(prov._id)},process.env.JWT_SECRET_KEY);
  // 3) verify the two calls the consent-filter plugin makes (the receipt build dereferences the SD via the facade)
  const c1=await cm('POST','/v1/users/identifier/search',{'x-visionstrust-consent-key':CK},{selfDescription:PROVIDER_SD,email:DID});
  const uid=JSON.parse(c1.body).userIdentifier;
  const c2=await cm('GET','/v1/consents/participants/'+uid+'?receipt=true',{authorization:'Bearer '+token});
  let st;try{st=JSON.parse(c2.body).consents.map(c=>c.status);}catch(e){st=c2.body.slice(0,200);}
  console.log('granted consent for '+DID);
  console.log('  provider org (TMForum): '+provOrg);
  console.log('  jwt-auth consumers (authority apisix): '+provConsumer+', '+consConsumer);
  console.log('  POST /v1/users/identifier/search      -> HTTP '+c1.code+'  (userIdentifier '+uid+')');
  console.log('  GET  /v1/consents/participants/<id>   -> HTTP '+c2.code+'  statuses='+JSON.stringify(st));
  console.log('  (the consent-filter plugin authenticates with clientID consent-demo-provider / clientSecret demo and derives the provider SD from /participants/me)');
  await mongoose.disconnect();
})().catch(e=>{console.error('ERR',e.stack||e.message);process.exit(1);});
NODE

JS="${JS//__DID__/$DID}"
B64=$(printf '%s' "$JS" | base64 -w0)
kubectl -n "$NS" exec "$POD" -- node -e "eval(Buffer.from('$B64','base64').toString('utf8'))"
