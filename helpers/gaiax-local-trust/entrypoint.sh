cp -R /development $OUTPUT_FOLDER/development
echo "selfSignedRoot: ${ROOT_CA}" > /data.yaml
mustache /data.yaml /trust-anchors.xml.mustache > $OUTPUT_FOLDER/development/trust-anchors/trust-anchors.xml