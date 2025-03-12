FROM toolbelt/mustache:2025-01-23

ENV ROOT_CA=x509RootValue
ENV OUTPUT_FOLDER=/out

USER 0


COPY ./data/development /development
COPY ./trust-anchors.xml.mustache /trust-anchors.xml.mustache
COPY ./entrypoint.sh /entrypoint.sh

ENTRYPOINT /entrypoint.sh