#!/bin/bash

/usr/lib/squid/security_file_certgen -c -s /var/lib/ssl_db -M 4MB
squid -N -d 1