FROM ubuntu:latest

COPY ./entrypoint.sh /entrypoint.sh
# Install dependencies
RUN apt update && apt install -y \
    squid-openssl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Open Squid port
EXPOSE 3128

RUN openssl req -new -newkey rsa:2048 -days 3650 -nodes -x509 -keyout /etc/squid/ca.pem -out /etc/squid/ca.pem -subj "/CN=Squid Proxy CA"

# Start Squid
ENTRYPOINT [ "/entrypoint.sh"]