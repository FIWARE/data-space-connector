# FIWARE Data Space Connector

## Overview
The FIWARE Data Space Connector enables trusted data exchange between participants in a data space. It provides a "Minimum Viable Dataspace" with two participants (consumer and provider), a trust anchor, and integration with TMForum APIs, Verifiable Credentials (OID4VP), and the Dataspace Protocol (DSP).

## Tech Stack
- Language: Java 17
- Build: Maven (multi-module)
- Framework: Cucumber 7.11.1 (BDD integration tests), OkHttp for HTTP clients, Keycloak for identity
- Test: JUnit 5 (via JUnit Platform Suite), Cucumber (Gherkin `.feature` files), Awaitility for async assertions
- Infrastructure: k3s (lightweight Kubernetes), Helm charts, Squid proxy (port 8888)

## Project Structure
```
pom.xml                          # Parent POM (packaging: pom, modules: [it])
it/                              # Integration test module
  pom.xml                        # IT module POM with Cucumber, OkHttp, Keycloak deps
  src/test/
    resources/
      it/mvds_basic.feature      # Cucumber feature file (2 active scenarios, 1 commented)
      policies/*.json            # ODRL policy JSON files used by step definitions
    java/org/fiware/dataspace/it/components/
      RunCucumberTest.java       # JUnit5 Suite entry point (@SelectClasspathResource("it"))
      StepDefinitions.java       # All Cucumber step definitions (~1005 lines)
      Wallet.java                # VC wallet: credential issuance, VP token creation, OID4VP exchange
      KeycloakHelper.java        # Keycloak token retrieval helper
      TestUtils.java             # HTTP client (OkHttp with Squid proxy), SSL trust-all, DID helpers
      MPOperationsEnvironment.java     # Provider environment constants (URLs, DIDs)
      FancyMarketplaceEnvironment.java # Consumer environment constants (URLs, DIDs)
      TrustAnchorEnvironment.java      # Trust anchor constants (TIR URL)
      model/                     # POJOs: TokenResponse, OpenIdConfiguration, DcatCatalog, etc.
doc/
  deployment-integration/local-deployment/LOCAL.MD  # Local deployment guide (~1500 lines)
  CENTRAL_MARKETPLACE.md         # Central marketplace integration guide (~333 lines)
  DSP_INTEGRATION.md             # Dataspace Protocol integration guide (~840 lines)
  scripts/                       # Shell scripts for credential/token operations
    get_credential.sh            # Get VC from Keycloak issuer
    get_access_token_oid4vp.sh   # Exchange VC for access token via OID4VP
    prepare-policies.sh          # Setup DSP policies
    prepare-central-market-policies.sh  # Setup central marketplace policies
    prepare-dsp-policies.sh      # Setup DSP-specific policies
    get-private-jwk-p-256.sh     # Extract private key as JWK
    get-participant-create.sh    # Generate participant creation payload for IdentityHub
    get-payload-from-jwt.sh      # Decode JWT payload
k3s/                             # Kubernetes deployment manifests
  provider.yaml                  # Provider deployment config
  consumer.yaml                  # Consumer deployment config
  trust-anchor.yaml              # Trust anchor deployment config
  dsp-provider.yaml              # DSP provider deployment config
  dsp-consumer.yaml              # DSP consumer deployment config
charts/                          # Helm charts
helpers/                         # Helper utilities (cert generation, etc.)
schema/                          # JSON schemas
```

## Build & Test
```bash
# Deploy local data space (basic)
mvn clean deploy -Plocal

# Deploy with central marketplace
mvn clean deploy -Plocal,central

# Deploy with DSP integration
mvn clean deploy -Plocal,dsp

# Run integration tests only (after deployment)
cd it && mvn verify

# The tests use Cucumber via JUnit Platform Suite
# Entry point: RunCucumberTest.java
# Feature files: src/test/resources/it/*.feature
```

## Key Conventions
- **BDD/Cucumber pattern**: Feature files in `src/test/resources/it/`, step definitions in `StepDefinitions.java`
- **Environment classes**: Abstract classes with static final URL/DID constants (e.g., `MPOperationsEnvironment`, `FancyMarketplaceEnvironment`, `TrustAnchorEnvironment`)
- **HTTP client**: All HTTP calls go through `TestUtils.OK_HTTP_CLIENT` (OkHttp with Squid proxy at localhost:8888 and trust-all SSL)
- **Wallet pattern**: `Wallet` class handles credential issuance from Keycloak and VP token exchange for access tokens
- **Policy loading**: Policies loaded from JSON resource files in `/policies/` directory via `getPolicy(name)` method
- **Cleanup**: `@Before`/`@After` hooks clean up policies, entities, TMForum resources, TIL entries, DCAT catalogs, and agreements
- **Awaitility**: Used for async assertions with configurable timeouts (typically 20-60 seconds)
- **Model POJOs**: Use Lombok `@Data` and Jackson annotations; ObjectMapper configured with `FAIL_ON_UNKNOWN_PROPERTIES=false`
- **TMForum models**: Generated from OpenAPI specs (in `org.fiware.dataspace.tmf.model` package)
- **Naming**: Step definitions use natural language matching the Gherkin steps exactly

## Important Files
- `it/src/test/resources/it/mvds_basic.feature` - Main (and currently only) feature file
- `it/src/test/java/org/fiware/dataspace/it/components/StepDefinitions.java` - All step implementations
- `it/src/test/java/org/fiware/dataspace/it/components/Wallet.java` - Credential wallet implementation
- `it/src/test/java/org/fiware/dataspace/it/components/TestUtils.java` - HTTP client and utilities
- `it/src/test/java/org/fiware/dataspace/it/components/MPOperationsEnvironment.java` - Provider URLs/DIDs
- `it/src/test/java/org/fiware/dataspace/it/components/FancyMarketplaceEnvironment.java` - Consumer URLs/DIDs
- `it/src/test/resources/policies/` - All ODRL policy JSON files
- `it/pom.xml` - IT module dependencies and build config
- `doc/scripts/get_credential.sh` - Script for getting VCs from Keycloak
- `doc/scripts/get_access_token_oid4vp.sh` - Script for OID4VP token exchange

## Key Service Endpoints (Local Deployment)
- **Trust Anchor TIR**: `http://tir.127.0.0.1.nip.io:8080`
- **Provider PAP**: `http://pap-provider.127.0.0.1.nip.io:8080`
- **Provider Data API**: `http://mp-data-service.127.0.0.1.nip.io:8080`
- **Provider TMF API (via PEP)**: `http://mp-tmf-api.127.0.0.1.nip.io:8080`
- **Provider TMF API (direct)**: `http://tm-forum-api.127.0.0.1.nip.io:8080`
- **Provider Scorpio (direct)**: `http://scorpio-provider.127.0.0.1.nip.io:8080`
- **Provider TIL (direct)**: `http://til-provider.127.0.0.1.nip.io:8080`
- **Provider Rainbow (direct)**: `http://rainbow-provider.127.0.0.1.nip.io:8080`
- **Consumer Keycloak**: `https://keycloak-consumer.127.0.0.1.nip.io:8443`
- **Provider Keycloak**: `https://keycloak-provider.127.0.0.1.nip.io:8443`
- **Central Marketplace**: `http://fancy-marketplace.127.0.0.1.nip.io:8080`
- **Contract Management**: `http://contract-management.127.0.0.1.nip.io:8080`
- **DSP DCP Management**: `http://dsp-dcp-management.127.0.0.1.nip.io:8080`
- **DSP OID4VC Management**: `http://dsp-oid4vc-management.127.0.0.1.nip.io:8080`
- **DSP Provider (DCP)**: `http://dcp-mp-operations.127.0.0.1.nip.io:8080`
- **DSP Provider (OID4VC)**: `http://dsp-mp-operations.127.0.0.1.nip.io:8080`
- **IdentityHub Consumer**: `http://identityhub-management-fancy-marketplace.127.0.0.1.nip.io:8080`
- **IdentityHub Provider**: `http://identityhub-management-mp-operations.127.0.0.1.nip.io:8080`
- **Vault Consumer**: `http://vault-fancy-marketplace.127.0.0.1.nip.io:8080`
- **Vault Provider**: `http://vault-mp-operations.127.0.0.1.nip.io:8080`
