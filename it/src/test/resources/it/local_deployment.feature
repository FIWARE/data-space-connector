@local
Feature: The local deployment should support credential issuance and authenticated data access as described in LOCAL.MD.

  # This feature covers the basic data exchange flow from LOCAL.MD:
  # - Trust anchor verification (both participants registered)
  # - Credential issuance via OID4VP Same-Device flow
  # - Policy creation and data entity creation
  # - Authenticated data access via OID4VP token exchange
  # - Unauthenticated access rejection
  # Note: Holder Verification and EIDAS compliancy are excluded per ticket requirements.

  Scenario: Trust anchor has both participants registered.
    Given The trust anchor TIR endpoint is available.
    Then M&P Operations DID is registered at the trust anchor.
    And Fancy Marketplace DID is registered at the trust anchor.
    And The trust anchor lists all registered issuers.

  Scenario: Consumer can retrieve credentials via Same-Device OID4VP flow.
    Given The consumer Keycloak credential issuer is configured.
    When The consumer employee logs into Keycloak and receives a user credential.
    Then The user credential is stored in the wallet.
    And The credential issuer metadata contains supported credential configurations.

  Scenario: Provider can create policies and data entities.
    When The provider creates an energy report access policy at the PAP.
    And The provider creates an EnergyReport entity at Scorpio.
    Then The energy report policy is active at the provider PAP.

  Scenario: Authenticated consumer can access provider data via OID4VP.
    Given M&P Operations is registered as a participant in the data space.
    And Fancy Marketplace is registered as a participant in the data space.
    When M&P Operations registers a policy to allow every participant access to its energy reports.
    And M&P Operations creates an energy report.
    And Fancy Marketplace issues a user credential to its employee.
    Then The provider data service exposes an openid-configuration endpoint.
    And Fancy Marketplace employee can exchange the credential for an access token.
    And Fancy Marketplace employee can access the EnergyReport with the access token.

  Scenario: Unauthenticated requests to the provider data service are rejected.
    Given M&P Operations is registered as a participant in the data space.
    When M&P Operations registers a policy to allow every participant access to its energy reports.
    And M&P Operations creates an energy report.
    Then An unauthenticated request to the provider data service returns 401.
