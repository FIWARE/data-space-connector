# Builds the Visions consent-manager (ISO/IEC TS 27560) for the FIWARE DSC.
#
# Productionizes the POC's build workaround (CONSENT_MANAGEMENT_PLAN.md §13.2 /
# backlog UF-12): the upstream `contract-agent` gitpkg dependency is paywalled, so
# we repoint it at the real VisionsOfficial/contract-consent-agent package (which
# ships a prebuilt dist). Builds with tsc (no tsc-at-startup) and runs as a
# non-root user (uid/gid 10100, matching the chart securityContext).
#
# Build args pin the sources:
#   CONSENT_MANAGER_REF  - VisionsOfficial/consent-manager git ref
#   CONTRACT_AGENT_REF   - VisionsOfficial/contract-consent-agent git ref

FROM node:22-bookworm-slim AS build

ARG CONSENT_MANAGER_REF=main
ARG CONTRACT_AGENT_REF=main
ENV MONGOMS_DISABLE_POSTINSTALL=1

RUN apt-get update \
 && apt-get install -y --no-install-recommends git openssl ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /usr/src/app

# 1) consent-manager source
RUN git clone --depth 1 --branch "${CONSENT_MANAGER_REF}" \
      https://github.com/VisionsOfficial/consent-manager.git .

# 2) UF-12: vendor the real contract-agent and repoint the dependency to it
RUN git clone --depth 1 --branch "${CONTRACT_AGENT_REF}" \
      https://github.com/VisionsOfficial/contract-consent-agent.git \
      vendor/contract-consent-agent \
 && node -e 'const f="package.json",j=require("./"+f);j.dependencies["contract-agent"]="file:./vendor/contract-consent-agent/contract-agent";require("fs").writeFileSync(f,JSON.stringify(j,null,2))'

# 3) install (npm resolves the file: dependency locally)
RUN npm install --unsafe-perm

# 4) build: drop the vendored agent's nested node_modules so its express/mongoose
#    types unify with the app's, then compile (skipLibCheck for remaining lib .d.ts)
RUN rm -rf vendor/contract-consent-agent/contract-agent/node_modules \
 && ./node_modules/.bin/tsc --skipLibCheck

# 5) signing keys (the app reads them from dist/src/config/keys at runtime)
RUN npm run generatePrivateKey \
 && npm run generateAES \
 && npm run generatePublicKey \
 && npm run copyKeys

# 6) the app insists a .env file exists (setupEnvironment throws otherwise), but
#    dotenv does NOT override real environment variables — so a placeholder file
#    satisfies the check while all config comes from the container environment.
RUN echo '# configuration is provided via the container environment (k8s)' > .env

# --- runtime ---------------------------------------------------------------
FROM node:22-bookworm-slim
ENV NODE_ENV=production
WORKDIR /usr/src/app

# non-root user matching the chart's securityContext (runAsUser/fsGroup 10100)
RUN groupadd -g 10100 app && useradd -u 10100 -g 10100 -m -s /usr/sbin/nologin app

COPY --from=build --chown=10100:10100 /usr/src/app /usr/src/app

USER 10100
EXPOSE 3000
CMD ["node", "dist/src/index.js"]
