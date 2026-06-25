@dsp
Feature: DSP Advanced Policy Scenarios as described in DSP_INTEGRATION.md.

  # This feature covers the "Advanced Policy Scenarios" section of DSP_INTEGRATION.md.
  # It exercises access policy filtering (Scenario A), contract policy rejection (Scenario B),
  # and explicit scope declarations in ODRL policies (Scenario C).

  Scenario: Scenario A - Restricted asset not visible due to access policy requiring PremiumPartnerCredential.
    Given The consumer identity is properly setup.
    And The provider identity is properly setup.
    And The provider catalog is properly setup.
    And The provider creates a restricted product spec with asset ASSET-2.
    And The provider creates a premium partner offering with restrictive access policy.
    When The consumer requests the provider catalog via the DCP management API.
    Then The DCP catalog does not contain asset ASSET-2.

  Scenario: Scenario B - Offer visible in catalog but contract negotiation rejected by contract policy.
    Given The consumer identity is properly setup.
    And The provider identity is properly setup.
    And The provider catalog is properly setup.
    And The provider creates a browse-only product spec with asset ASSET-3.
    And The provider creates a browse-only offering with restrictive contract policy.
    When The consumer requests the provider catalog via the DCP management API.
    Then The DCP catalog contains asset ASSET-3.
    When The consumer starts a contract negotiation for asset ASSET-3.
    Then The contract negotiation for asset ASSET-3 is terminated.

  Scenario: Scenario C - Explicit dspace:scope in contract policy permissions works for negotiation and transfer.
    Given The consumer identity is properly setup.
    And The provider identity is properly setup.
    And The provider catalog is properly setup.
    And The provider creates a scoped-policy product spec with asset ASSET-4.
    And The provider creates an offering with explicit scope declarations in contract policy.
    And The provider creates an UptimeReport entity in Scorpio for the DSP offering.
    When The consumer requests the provider catalog via the DCP management API.
    Then The DCP catalog contains asset ASSET-4.
    When The consumer negotiates a contract for asset ASSET-4 via DCP.
    And The consumer waits for the ASSET-4 negotiation to be finalized.
    And The consumer starts a transfer for asset ASSET-4 via DCP.
    And The consumer waits for the ASSET-4 transfer to start.
    Then The consumer retrieves the EDR and accesses UptimeReport via the ASSET-4 transfer.
