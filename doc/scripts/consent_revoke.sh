#!/usr/bin/env bash
#
# consent_revoke.sh -- withdraw the demo consent for a holder DID in the (POC)
# consent-manager by setting the consent record's status to `revoked`. The
# consent-filter plugin only allows when a consent is `granted`, so access is
# denied again on the next request. Counterpart of consent_grant.sh.
#
# The consent-manager runs in the trust-anchor namespace (central authority).
#
# Usage: ./doc/scripts/consent_revoke.sh <holder-did> [namespace]
#   namespace : kubernetes namespace of the consent-manager (default: trust-anchor)
set -euo pipefail

DID="${1:?usage: consent_revoke.sh <holder-did> [namespace]}"
NS="${2:-trust-anchor}"

POD=$(kubectl -n "$NS" get pods -o name 2>/dev/null | grep consent-manager | head -1 | cut -d/ -f2 || true)
[ -n "$POD" ] || { echo "consent-manager pod not found in namespace '$NS'" >&2; exit 1; }

read -r -d '' JS <<'NODE' || true
const NM='/usr/src/app/node_modules/';
const mongoose=require(NM+'mongoose');
const UserIdentifier=require('/usr/src/app/dist/src/models/UserIdentifier/UserIdentifier.model').default;
const Consent=require('/usr/src/app/dist/src/models/Consent/Consent.model').default;
const DID='__DID__';
(async()=>{
  await mongoose.connect(process.env.MONGO_URI);
  const uis=await UserIdentifier.find({email:DID}).select('_id');
  const ids=uis.map(u=>u._id);
  const r=await Consent.updateMany({providerUserIdentifier:{$in:ids},status:'granted'},{$set:{status:'revoked',consented:false}});
  console.log('revoked '+ (r.modifiedCount!==undefined?r.modifiedCount:r.nModified) +' consent(s) for '+DID);
  await mongoose.disconnect();
})().catch(e=>{console.error('ERR',e.stack||e.message);process.exit(1);});
NODE

JS="${JS//__DID__/$DID}"
B64=$(printf '%s' "$JS" | base64 -w0)
kubectl -n "$NS" exec "$POD" -- node -e "eval(Buffer.from('$B64','base64').toString('utf8'))"
