@dsp
Feature: DSP DCP protocol flow as described in DSP_INTEGRATION.md.

  # This feature covers the "Order through DSP" > "DCP" section of DSP_INTEGRATION.md.
  # It exercises the full DCP-based Dataspace Protocol flow: catalog browsing, contract
  # negotiation, transfer process, EDR retrieval, and data access.

  Scenario: Consumer reads the provider catalog via DCP management API.
    Given The consumer identity is properly setup.
    And The provider identity is properly setup.
    And The provider catalog is properly setup.
    When The consumer requests the provider catalog via the DCP management API.
    Then The DCP catalog contains at least one dataset with the DSP asset.

  Scenario: Consumer negotiates a contract via DCP and retrieves the agreement.
    Given The consumer identity is properly setup.
    And The provider identity is properly setup.
    And The provider catalog is properly setup.
    When The consumer starts a contract negotiation via the DCP management API.
    And The consumer waits for the DCP negotiation to be finalized.
    Then The DCP negotiation yields a valid contract agreement ID.

  Scenario: Consumer starts a transfer process via DCP and retrieves data.
    Given The consumer identity is properly setup.
    And The provider identity is properly setup.
    And The provider catalog is properly setup.
    And The provider creates an UptimeReport entity in Scorpio for the DSP offering.
    And The consumer has a finalized DCP contract agreement.
    When The consumer starts a transfer process via the DCP management API.
    And The consumer waits for the DCP transfer process to start.
    Then The consumer retrieves the EDR data address from the DCP management API.
    And The consumer accesses the UptimeReport entity via the DCP transfer endpoint.
