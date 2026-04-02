# Implementation Plan: [DSC] Extend integration-testing to cover new features

## Overview
Extend the existing Cucumber/BDD integration test framework in the `it/` directory to cover all documented workflows from LOCAL.MD (basic data exchange, marketplace buying, cluster creation), CENTRAL_MARKETPLACE.md (central marketplace flows with `mvn clean deploy -Plocal,central`), and DSP_INTEGRATION.md (Dataspace Protocol flows with `mvn clean deploy -Plocal,dsp`). This involves adding new feature files, environment classes, step definitions, model POJOs, and policy resources for each deployment profile.

## Steps

### Step 1: Add environment classes for Central Marketplace and DSP profiles

Add new environment configuration classes that define the service endpoints specific to the `central` and `dsp` Maven profiles. These are analogous to the existing `MPOperationsEnvironment` and `FancyMarketplaceEnvironment` but contain URLs for the new services.

**Files to create:**
- `it/src/test/java/org/fiware/dataspace/it/components/CentralMarketplaceEnvironment.java` — Constants for central marketplace endpoints: `http://fancy-marketplace.127.0.0.1.nip.io:8080` (TMForum/marketplace API), `http://contract-management.127.0.0.1.nip.io:8080` (contract management)
- `it/src/test/java/org/fiware/dataspace/it/components/DSPEnvironment.java` — Constants for DSP endpoints: DCP management API (`http://dsp-dcp-management.127.0.0.1.nip.io:8080`), OID4VC management API (`http://dsp-oid4vc-management.127.0.0.1.nip.io:8080`), DCP provider (`http://dcp-mp-operations.127.0.0.1.nip.io:8080`), OID4VC provider (`http://dsp-mp-operations.127.0.0.1.nip.io:8080`), IdentityHub management endpoints for consumer and provider, Vault endpoints for consumer and provider

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/MPOperationsEnvironment.java` — Add provider keycloak address constant (`https://keycloak-provider.127.0.0.1.nip.io`)

**Acceptance criteria:**
- All new URL constants match exactly the endpoints documented in CENTRAL_MARKETPLACE.md and DSP_INTEGRATION.md
- Each class is abstract with static final fields, following the pattern of existing environment classes
- Every public field has a Javadoc comment

### Step 2: Add model POJOs for DSP protocol interactions

Add new model POJOs needed for DSP catalog, contract negotiation, and transfer process interactions that don't exist yet in the `model/` package.

**Files to create in `it/src/test/java/org/fiware/dataspace/it/components/model/`:**
- `CatalogRequestMessage.java` — Request body for DSP catalog requests (context, type, protocol, counterPartyId, counterPartyAddress, querySpec)
- `ContractRequest.java` — Request body for DSP contract negotiation (context, type, counterPartyAddress, counterPartyId, protocol, policy)
- `ContractNegotiation.java` — Response from contract negotiation queries (id, state, contractAgreementId)
- `TransferProcess.java` — Response from transfer process queries (id, state)
- `DataAddress.java` — Response from EDR data address queries (endpoint, token)
- `ParticipantCreate.java` — Request body for IdentityHub participant registration

**Acceptance criteria:**
- All POJOs use Lombok `@Data` annotation and Jackson annotations for deserialization
- Fields match the JSON structures in the DSP_INTEGRATION.md documentation
- ObjectMapper with `FAIL_ON_UNKNOWN_PROPERTIES=false` can deserialize all response types

### Step 3: Extend the Wallet class with provider credential support

The current `Wallet` class only supports consumer credential issuance. Extend it to also support getting credentials from the provider's Keycloak instance, and add a method for SD-JWT credential format (needed for central marketplace flows).

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/Wallet.java` — Add method `getCredentialFromIssuer(String userToken, String issuerHost, String credentialId, String format)` that supports both `jwt_vc` and `vc+sd-jwt` formats. The existing method defaults to `jwt_vc`.

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/FancyMarketplaceEnvironment.java` — Add `REPRESENTATIVE_USER_NAME = "representative@consumer.org"` constant for the representative user role used in marketplace flows

**Acceptance criteria:**
- Wallet can issue credentials from both consumer and provider Keycloak instances
- SD-JWT format credential issuance is supported for the central marketplace flow
- Existing functionality is not broken

### Step 4: Add helper class for DSP IdentityHub operations

Create a helper class that encapsulates the IdentityHub and Vault operations needed for DSP test setup (inserting keys, registering participants, inserting credentials). These operations are documented in the "Setup the consumer/provider" sections of DSP_INTEGRATION.md.

**Files to create:**
- `it/src/test/java/org/fiware/dataspace/it/components/IdentityHubHelper.java` — Methods:
  - `insertKeyIntoVault(String vaultAddress, String keyAlias, String jwk)` — POST to Vault secret endpoint
  - `registerParticipant(String identityHubManagementAddress, String participantJson)` — POST to IdentityHub participants API
  - `insertCredential(String identityHubManagementAddress, String participantId, String credentialId, String rawVc, String credentialContent)` — POST credential to IdentityHub
  - `getPrivateKeyAsJwk(String pemKeyPath)` — Convert PEM private key to JWK format (equivalent to `get-private-jwk-p-256.sh`)
  - `buildParticipantPayload(String jwk, String did, String identityHubAddress, String keyAlias)` — Build participant creation JSON (equivalent to `get-participant-create.sh`)

**Files to create:**
- `it/src/test/java/org/fiware/dataspace/it/components/ScriptHelper.java` — Java equivalents of the shell scripts in `doc/scripts/` needed by the tests: `get_access_token_oid4vp.sh`, `get_credential.sh`, `get-payload-from-jwt.sh`. This avoids shell script dependencies in Java tests.

**Acceptance criteria:**
- All IdentityHub operations match the curl commands documented in DSP_INTEGRATION.md
- API key header (`x-api-key: c3VwZXItdXNlcg==.random`) and Vault token (`X-Vault-Token: root`) are configurable constants
- Every public method has Javadoc documentation

### Step 5: Add helper class for DSP Management API operations

Create a helper class for interacting with the FDSC-EDC Management API (catalog requests, contract negotiations, transfer processes, EDR retrieval). These are the core DSP protocol operations.

**Files to create:**
- `it/src/test/java/org/fiware/dataspace/it/components/DSPManagementHelper.java` — Methods:
  - `requestCatalog(String managementApiAddress, String counterPartyId, String counterPartyAddress)` — POST catalog request
  - `startNegotiation(String managementApiAddress, String counterPartyAddress, String counterPartyId, String offerId, String assetId, Object policy)` — POST contract negotiation
  - `getNegotiations(String managementApiAddress)` — POST query for negotiation state
  - `waitForNegotiationFinalized(String managementApiAddress)` — Poll until negotiation state is "finalized", return agreement ID (using Awaitility)
  - `startTransferProcess(String managementApiAddress, String assetId, String counterPartyId, String counterPartyAddress, String contractId, String transferType)` — POST transfer process
  - `getTransferProcesses(String managementApiAddress)` — POST query for transfer processes
  - `waitForTransferCompleted(String managementApiAddress)` — Poll until transfer is started, return transfer ID
  - `getDataAddress(String managementApiAddress, String transferId)` — GET EDR data address (endpoint + token)

**Acceptance criteria:**
- All API calls match the curl commands in DSP_INTEGRATION.md exactly
- Uses `TestUtils.OK_HTTP_CLIENT` for HTTP calls
- Awaitility-based polling methods use configurable timeouts
- Supports both DCP and OID4VC management API endpoints (parameterized by address)

### Step 6: Create feature file and step definitions for LOCAL.MD credential issuance and data access

Create a new feature file covering the LOCAL.MD basic data exchange flow: trust anchor verification, policy creation, data entity creation, credential issuance via OID4VP Same-Device flow, and authenticated data access. Much of this is already covered by the existing `mvds_basic.feature` Scenario 1, but this step ensures full coverage of all LOCAL.MD steps.

**Files to create:**
- `it/src/test/resources/it/local_deployment.feature` — Feature file with scenarios:
  - "Trust anchor has both participants registered" — Verify both DIDs at TIR
  - "Consumer can retrieve credentials via Same-Device flow" — Full OID4VP credential issuance flow (Keycloak login -> offer URI -> pre-authorized code -> credential access token -> credential)
  - "Provider can create policies and data entities" — Policy creation at PAP, entity creation at Scorpio
  - "Authenticated consumer can access provider data" — Full OID4VP authentication and data retrieval
  - "Unauthenticated requests are rejected" — Verify 401 without token

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/StepDefinitions.java` — Add new step definitions for LOCAL.MD-specific steps not yet covered (e.g., verifying credential issuance details, checking well-known endpoints, verifying 401 on unauthenticated access)

**Acceptance criteria:**
- Feature file covers all LOCAL.MD steps except Holder Verification and EIDAS compliancy (as specified in ticket)
- Steps are specific and descriptive using Given/When/Then BDD pattern
- All step definitions reuse existing helper classes (Wallet, KeycloakHelper, environment classes)

### Step 7: Create feature file and step definitions for LOCAL.MD marketplace and service buying flow

Create a feature file covering the LOCAL.MD "Buy access to a service offering" flow: offer creation (product specification with credentials config and policy config for small/full), self-registration, buying access via TMForum, and verifying access is granted.

**Files to create:**
- `it/src/test/resources/it/local_marketplace.feature` — Feature file with scenarios:
  - "Provider creates product offerings with credential and policy configuration" — Create category, catalog, product specs (small and full), and product offerings with ODRL policies
  - "Consumer self-registers at provider marketplace" — Register organization via TMForum API with authentication
  - "Consumer buys access to provider offering and gains access" — Full order flow: list offerings, create order, complete order, verify credential access is granted
  - "Consumer with restricted offering can only perform allowed operations" — Verify small-tier restrictions vs full-tier

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/StepDefinitions.java` — Add new step definitions specific to the LOCAL.MD marketplace flow that aren't covered by existing steps (product spec creation with credentialsConfig + policyConfig characteristics, small vs full spec comparison, order completion verification)

**Files to create:**
- `it/src/test/resources/policies/allowSelfRegistrationLegalPerson.json` — Policy for legal person self-registration (if not already present; verify content matches LOCAL.MD docs)

**Acceptance criteria:**
- Feature file covers the complete marketplace buying flow from LOCAL.MD
- Product specification creation includes both credentialsConfiguration and authorizationPolicy characteristics
- Order completion triggers contract management and verifiably creates TIL entries and PAP policies
- Both small and full product specification variants are tested

### Step 8: Create feature file and step definitions for CENTRAL_MARKETPLACE.md flows

Create a feature file covering the central marketplace integration flows: marketplace preparation, provider registration at central marketplace, product offering creation, customer buying, and contract management notification verification.

**Files to create:**
- `it/src/test/resources/it/central_marketplace.feature` — Feature file with scenarios:
  - "Central marketplace is prepared with access policies" — Register TMForum access policies (REPRESENTATIVE role restriction)
  - "Provider prepares for central marketplace integration" — Create contract management access policy, get provider credential, register provider organization at marketplace with contract management address and clientId/scope
  - "Provider creates product offering on central marketplace" — Create product spec with credentials config and policies, create product offering referencing the provider
  - "Consumer buys access through central marketplace" — Get consumer credentials (UserCredential as SD-JWT, OperatorCredential), verify access not yet possible, register at marketplace, list offerings, create and complete order
  - "Contract management notification flow activates service" — After order completion, verify that the marketplace's contract management sends notification to provider's contract management, which creates TIL entry and PAP policy, verify consumer can now access with OperatorCredential

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/StepDefinitions.java` or create a new step definitions class:
- `it/src/test/java/org/fiware/dataspace/it/components/CentralMarketplaceStepDefinitions.java` — New step definitions class for central marketplace scenarios. This keeps the code modular. Steps include:
  - Central marketplace policy preparation
  - Provider registration at central marketplace (with contract management config)
  - Product offering creation at central marketplace
  - Consumer order and access verification through central marketplace
  - Contract management notification verification

**Acceptance criteria:**
- Feature file covers all steps in CENTRAL_MARKETPLACE.md
- Tests use the `local,central` profile endpoints (CentralMarketplaceEnvironment)
- Contract management notification flow is verified end-to-end (order completion -> TIL entry + PAP policy creation)
- SD-JWT credential format is supported for consumer credentials

### Step 9: Create feature file and step definitions for DSP_INTEGRATION.md identity setup

Create a feature file covering the DSP identity setup: consumer and provider identity registration in IdentityHub, key insertion into Vault, membership credential issuance, and trusted issuers list verification.

**Files to create:**
- `it/src/test/resources/it/dsp_identity_setup.feature` — Feature file with scenarios:
  - "Consumer identity is registered in IdentityHub" — Get consumer private key as JWK, insert into Vault, register participant in IdentityHub, verify DID document is available
  - "Provider identity is registered in IdentityHub" — Same flow for provider
  - "Membership credentials are issued and stored" — Get membership credentials from Keycloak for both consumer and provider, insert into IdentityHub
  - "Trusted issuers list is configured for both participants" — Verify both participants trust each other for membership credentials

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/DSPStepDefinitions.java` (new file) — Step definitions for DSP identity setup using `IdentityHubHelper`

**Acceptance criteria:**
- All identity setup steps from DSP_INTEGRATION.md "Setup the consumer" and "Setup the provider" sections are covered
- DID documents are verifiable via `.well-known/did.json` endpoints
- Membership credentials are correctly stored in IdentityHub

### Step 10: Create feature file and step definitions for DSP_INTEGRATION.md offering and TMForum ordering

Create a feature file covering the DSP offering creation and TMForum-based ordering flow from DSP_INTEGRATION.md.

**Files to create:**
- `it/src/test/resources/it/dsp_tmforum_ordering.feature` — Feature file with scenarios:
  - "Provider creates demo data" — Insert UptimeReport entity into Scorpio
  - "Provider creates DSP-enabled product offering" — Create category, catalog, product spec (with DCP/OID4VC endpoints, upstream address, target specification, service configuration, credentials config, policy config), and product offering (with EDC contract definition terms including access and contract policies)
  - "Policies are prepared for TMForum access" — Setup access policies via `prepare-policies.sh` equivalent
  - "Consumer orders through TMForum" — Get representative and operator credentials, register consumer organization, list offerings, create and complete order
  - "Consumer can access data after TMForum order" — Verify operator credential grants access to uptime report via OID4VP

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/DSPStepDefinitions.java` — Add step definitions for DSP offering and TMForum ordering

**Acceptance criteria:**
- Product specification includes all DSP-specific characteristics (dcp endpoint, oid4vc endpoint, upstreamAddress, targetSpecification, serviceConfiguration, credentialsConfig, policyConfig)
- Product offering includes EDC contract definition terms (accessPolicy, contractPolicy)
- Order completion triggers contract management and enables access

### Step 11: Create feature file and step definitions for DSP_INTEGRATION.md DCP protocol flow

Create a feature file covering the full DCP-based DSP flow: catalog browsing, contract negotiation, transfer process, and data access via DCP authentication.

**Files to create:**
- `it/src/test/resources/it/dsp_dcp_flow.feature` — Feature file with scenarios:
  - "Consumer can read provider catalog via DCP" — Request catalog from DCP management API, verify offering is listed
  - "Consumer negotiates contract via DCP" — Start negotiation with offer, poll until finalized, retrieve agreement ID
  - "Consumer starts transfer process via DCP" — Request transfer with agreement ID, poll until started, retrieve transfer ID
  - "Consumer retrieves data endpoint and token via DCP" — Get EDR data address, extract endpoint URL and access token
  - "Consumer accesses data service via DCP transfer" — Use endpoint and token to access uptime report entity

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/DSPStepDefinitions.java` — Add step definitions for DCP flow using `DSPManagementHelper`

**Acceptance criteria:**
- All DCP flow steps from DSP_INTEGRATION.md are covered end-to-end
- Catalog request uses correct protocol (`dataspace-protocol-http:2025-1`) and counter-party details
- Negotiation polling correctly waits for "finalized" state
- Transfer process uses `HttpData-PULL` transfer type
- Data access through the provisioned endpoint returns the expected entity

### Step 12: Create feature file and step definitions for DSP_INTEGRATION.md OID4VC protocol flow

Create a feature file covering the OID4VC-based DSP flow: catalog browsing, transfer process setup reusing DCP agreement, and data access via OID4VP authentication.

**Files to create:**
- `it/src/test/resources/it/dsp_oid4vc_flow.feature` — Feature file with scenarios:
  - "Consumer can read provider catalog via OID4VC" — Request catalog from OID4VC management API, verify offering is listed
  - "Consumer starts transfer process via OID4VC reusing DCP agreement" — Request transfer with existing agreement ID (from DCP flow), poll until started
  - "Consumer cannot access without authentication via OID4VC transfer" — Verify unauthenticated request fails
  - "Consumer retrieves openid-configuration for OID4VC transfer endpoint" — Verify well-known endpoint is available
  - "Consumer accesses data service via OID4VP authentication" — Get membership credential, exchange for access token via OID4VP, access uptime report

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/DSPStepDefinitions.java` — Add step definitions for OID4VC flow

**Acceptance criteria:**
- Catalog request via OID4VC management API returns same offerings as DCP
- Transfer process reuses agreement from DCP negotiation (as documented)
- OID4VP authentication flow works: get membership credential -> exchange for token -> access data
- Unauthenticated access returns 401/403

### Step 13: Add Cucumber tags and test configuration for profile-based test execution

Configure the test infrastructure so that tests can be selectively run based on the active Maven profile (`local`, `central`, `dsp`). This prevents tests for the central marketplace from running when only the local profile is deployed.

**Files to modify:**
- `it/src/test/resources/it/local_deployment.feature` — Add tag `@local`
- `it/src/test/resources/it/local_marketplace.feature` — Add tag `@local`
- `it/src/test/resources/it/mvds_basic.feature` — Add tag `@local`
- `it/src/test/resources/it/central_marketplace.feature` — Add tag `@central`
- `it/src/test/resources/it/dsp_identity_setup.feature` — Add tag `@dsp`
- `it/src/test/resources/it/dsp_tmforum_ordering.feature` — Add tag `@dsp`
- `it/src/test/resources/it/dsp_dcp_flow.feature` — Add tag `@dsp`
- `it/src/test/resources/it/dsp_oid4vc_flow.feature` — Add tag `@dsp`
- `it/pom.xml` — Add Maven profiles (`local-test`, `central-test`, `dsp-test`) that set the `cucumber.filter.tags` system property for the failsafe plugin
- `it/src/test/java/org/fiware/dataspace/it/components/RunCucumberTest.java` — Optionally add a `@ConfigurationParameter` for tag filtering via system property

**Acceptance criteria:**
- Running `mvn verify -Plocal-test` in the `it/` module only executes `@local` tagged tests
- Running `mvn verify -Pcentral-test` executes `@central` tagged tests
- Running `mvn verify -Pdsp-test` executes `@dsp` tagged tests
- Running `mvn verify` without a specific test profile runs all tests
- Tags are documented in the feature files

### Step 14: Add cleanup and setup hooks for Central Marketplace and DSP tests

Add proper `@Before`/`@After` hooks for the central marketplace and DSP test scenarios to ensure test isolation. The existing cleanup in `StepDefinitions.java` handles local profile resources; new hooks need to handle central marketplace and DSP-specific resources.

**Files to modify:**
- `it/src/test/java/org/fiware/dataspace/it/components/CentralMarketplaceStepDefinitions.java` — Add `@Before("@central")` and `@After("@central")` hooks that clean up:
  - Central marketplace TMForum resources (organizations, offerings, orders, specs)
  - Provider contract management policies
  - Consumer TIL entries at provider side

- `it/src/test/java/org/fiware/dataspace/it/components/DSPStepDefinitions.java` — Add `@Before("@dsp")` and `@After("@dsp")` hooks that clean up:
  - IdentityHub participants and credentials
  - Vault keys
  - TMForum resources (categories, catalogs, specs, offerings, orders)
  - DSP management API resources (negotiations, transfers)
  - DCAT catalogs and agreements at Rainbow
  - PAP policies
  - Scorpio entities

**Acceptance criteria:**
- Tests can run in any order without side effects
- Cleanup handles partial failures gracefully (try-catch with logging)
- Setup hooks verify prerequisite services are available before running tests

### Step 15: Review, documentation, and verification

Final review step to ensure all feature files are consistent, well-documented, and properly integrated.

**Tasks:**
- Verify all feature files follow consistent Gherkin style and naming conventions
- Verify all step definitions have unique regex patterns (no ambiguous step matching)
- Verify all new policy JSON files (if any) are valid ODRL
- Verify all model POJOs deserialize correctly with test data
- Add inline documentation to all new step definitions explaining what each step verifies
- Review that the commented-out Scenario 3 in `mvds_basic.feature` ("Transfer Process Protocol") can be updated to use the new DSP infrastructure or properly replaced by the new DSP feature files
- Ensure no magic constants — all URLs, DIDs, credentials IDs, and timeouts are defined as named constants in environment/helper classes

**Acceptance criteria:**
- All new code follows existing project conventions
- No duplicate step definitions across step definition classes
- All public methods have Javadoc documentation
- No magic constants in step definitions
- The commented-out scenario in `mvds_basic.feature` is either updated or explicitly superseded by the new DSP feature files with a comment explaining the relationship
