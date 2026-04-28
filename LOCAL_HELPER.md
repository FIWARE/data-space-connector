kubectl get secret ca-secret -n cert-manager -o jsonpath='{.data.tls\.crt}' | base64 -d > local-ca.crt 
  open chrome://settings/certificates → "Authorities" → Import local-ca.crt and tick "Trust this certificate for identifying websites". Same effect, just the UI path.
