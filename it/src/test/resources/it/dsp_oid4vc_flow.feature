@dsp
Feature: DSP OID4VC protocol flow as described in DSP_INTEGRATION.md.

  # This feature covers the "Order through DSP" > "OID4VC" section of DSP_INTEGRATION.md.
  # It exercises the OID4VC-based Dataspace Protocol flow: catalog browsing via the OID4VC
  # management API, transfer process setup reusing a DCP agreement, unauthenticated access
  # rejection, openid-configuration verification, and OID4VP-authenticated data access.
  #
  # As documented, the OID4VC flow reuses the agreement negotiated via DCP rather than
  # running a full negotiation again - an agreement can be used independently from the
  # protocol it was negotiated under.

  Scenario: Consumer reads the provider catalog via OID4VC management API.
    Given The consumer identity is properly setup.
    And The provider identity is properly setup.
    And The provider catalog is properly setup.
    When The consumer requests the provider catalog via the OID4VC management API.
    Then The OID4VC catalog contains at least one dataset with the DSP asset.


  Scenario: Consumer negotiates a contract via OID4VC and retrieves the agreement.
    Given The consumer identity is properly setup.
    And The provider identity is properly setup.
    And The provider catalog is properly setup.
    When The consumer starts a contract negotiation via the OID4VC management API.
    And The consumer waits for the OID4VC negotiation to be finalized.
    Then The OID4VC negotiation yields a valid contract agreement ID.

  Scenario: Consumer cannot access data without authentication via OID4VC transfer.
    Given An OID4VC Transfer Process is started.
    And The consumer retrieves the EDR data address from the OID4VC management API.
    When The consumer requests the UptimeReport without authentication via the OID4VC endpoint.
    Then The unauthenticated OID4VC request is rejected.

  Scenario: Consumer verifies openid-configuration is available at the OID4VC transfer endpoint.
    Given An OID4VC Transfer Process is started.
    And The consumer retrieves the EDR data address from the OID4VC management API.
    When The consumer requests the openid-configuration from the OID4VC transfer endpoint.
    Then The openid-configuration response contains a token endpoint.

  Scenario: Consumer accesses data via OID4VP authentication through OID4VC transfer.
    Given An OID4VC Transfer Process is started.
    And The consumer retrieves the EDR data address from the OID4VC management API.
    When The consumer obtains a membership credential for OID4VC access.
    And The consumer exchanges the membership credential for an access token via OID4VP at the OID4VC endpoint.
    Then The consumer accesses the UptimeReport entity via the OID4VC transfer endpoint with OID4VP token.
