@dsp
Feature: DSP TMForum ordering flow as described in DSP_INTEGRATION.md.

  # This feature covers the "Prepare some data", "Prepare the offering",
  # "Order through TMForum", and data access verification sections
  # of DSP_INTEGRATION.md.

  Scenario: Provider creates demo data for the DSP offering.
    When The provider creates an UptimeReport entity in Scorpio for the DSP offering.
    Then The UptimeReport entity exists in Scorpio.

  Scenario: Provider creates a DSP-enabled product offering via TMForum.
    Given The provider creates a demo category for DSP offerings.
    And The provider creates a demo catalog for DSP offerings.
    When The provider creates a DSP product specification with DCP and OID4VC endpoints.
    And The provider creates a DSP product offering with EDC contract definition terms.
    Then The DSP product offering is available at the TMForum API.

  Scenario: Policies are prepared for TMForum access in DSP deployment.
    When The provider registers the product offering read policy for DSP.
    And The provider registers the self-registration policy for DSP.
    And The provider registers the product ordering policy for DSP.
    Then The TMForum access policies are active at the provider PAP.

  Scenario: Consumer orders through TMForum in the DSP deployment.
    Given The provider creates demo data and offering for DSP.
    And The DSP TMForum access policies are in place.
    And The consumer identity is properly setup.
    And The provider identity is properly setup.
    When The consumer obtains a representative credential for DSP ordering.
    And The consumer obtains an operator credential for DSP ordering.
    And The consumer registers at the provider marketplace for DSP.
    And The consumer lists offerings and creates a product order for DSP.
    And The consumer completes the product order for DSP.
    Then The consumer can access the UptimeReport with the operator credential.
