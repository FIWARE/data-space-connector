@eidas
Feature: The eIDAS deployment issues credentials bound to an eIDAS certificate and verifies them against a trust list.

  # This feature covers the eIDAS 2.0 flow from doc/deployment-integration/eidas/README.md:
  # - the issuer is identified by a did:elsi DID rather than a did:web one
  # - issued credentials carry the certificate chain in the x5c JOSE header
  # - the verifier validates that chain against the EU-trust-list format (a mock list locally)
  #   and matches the certificate's organizationIdentifier against the DID
  # Everything else - policies, entities, data access - is the standard OID4VP flow.

  Scenario: The eIDAS issuer is registered at the trust anchor.
    Given The trust anchor TIR endpoint is available.
    Then The did:elsi issuer is registered at the trust anchor.

  Scenario: Issued credentials carry the eIDAS certificate chain.
    Given The eIDAS consumer Keycloak credential issuer is configured.
    When The eIDAS consumer employee receives a user credential.
    Then The credential is issued by the did:elsi issuer.
    And The credential carries the eIDAS certificate chain in its x5c header.
    And The signing certificate's organizationIdentifier matches the did:elsi issuer.
    And The credential is signed as JAdES.

  Scenario: The certificate chain terminates in a CA the verifier trusts.
    Given The eIDAS consumer Keycloak credential issuer is configured.
    When The eIDAS consumer employee receives a user credential.
    Then The certificate chain of the credential is complete.

  Scenario: A credential bound to an eIDAS certificate grants access to provider data.
    Given The did:elsi issuer is trusted by the provider.
    When M&P Operations registers a policy to allow every participant access to its energy reports.
    And M&P Operations creates an energy report.
    And The eIDAS consumer employee receives a user credential.
    Then The provider data service exposes an openid-configuration endpoint.
    And The eIDAS credential can be exchanged for an access token.
    And The eIDAS consumer employee can access the EnergyReport with the access token.

  Scenario: Unauthenticated requests to the provider data service are rejected.
    When M&P Operations registers a policy to allow every participant access to its energy reports.
    And M&P Operations creates an energy report.
    Then An unauthenticated request to the provider data service returns 401.
