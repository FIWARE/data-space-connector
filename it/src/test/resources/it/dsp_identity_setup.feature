@dsp
Feature: DSP identity setup for consumer and provider participants as described in DSP_INTEGRATION.md.

  # This feature covers the "Setup the consumer", "Setup the provider",
  # "Issue membership-credentials", and "Trusted Issuers List" sections
  # of DSP_INTEGRATION.md.

  Scenario: Consumer identity is registered in IdentityHub.
    Given The consumer private key is retrieved from the Kubernetes signing-key secret.
    When The consumer private key is converted to JWK format.
    And The consumer JWK is inserted into the consumer Vault.
    And The consumer participant is registered in the consumer IdentityHub.
    Then The consumer DID document is available at the well-known endpoint.

  Scenario: Provider identity is registered in IdentityHub.
    Given The provider private key is retrieved from the Kubernetes signing-key secret.
    When The provider private key is converted to JWK format.
    And The provider JWK is inserted into the provider Vault.
    And The provider participant is registered in the provider IdentityHub.
    Then The provider DID document is available at the well-known endpoint.

  Scenario: Membership credentials are issued and stored in IdentityHub.
    Given The consumer identity is registered in IdentityHub.
    And The provider identity is registered in IdentityHub.
    When A membership credential is issued for the consumer from the consumer Keycloak.
    And The consumer membership credential is inserted into the consumer IdentityHub.
    And A membership credential is issued for the provider from the provider Keycloak.
    And The provider membership credential is inserted into the provider IdentityHub.
    Then The consumer membership credential is stored in the consumer IdentityHub.
    And The provider membership credential is stored in the provider IdentityHub.

  Scenario: Trusted issuers list is configured for both participants.
    Given M&P Operations is registered as a participant in the data space.
    And Fancy Marketplace is registered as a participant in the data space.
    Then The provider trusted issuers list contains the consumer DID.
    And The consumer is trusted for membership credentials at the provider.
