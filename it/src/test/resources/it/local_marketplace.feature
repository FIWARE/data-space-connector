@local
Feature: The local deployment should support marketplace buying and service access as described in LOCAL.MD.

  # This feature covers the LOCAL.MD "Buy access to a service offering" flow:
  # - Provider creates product offerings with credential and policy configuration (small and full variants)
  # - Consumer self-registers at provider marketplace
  # - Consumer buys access to provider offering and gains access
  # - Consumer with restricted (small) offering can only perform allowed operations
  # Note: Holder Verification and EIDAS compliancy are excluded per ticket requirements.

  Scenario: Provider creates product offerings with credential and policy configuration.
    Given M&P Operations is registered as a participant in the data space.
    When The provider creates a K8S Small product specification with credentials and policy config.
    And The provider creates a K8S Full product specification with credentials and policy config.
    And The provider creates a Small product offering referencing the Small specification.
    And The provider creates a Full product offering referencing the Full specification.
    Then Two product offerings are available at the provider TMForum API.

  Scenario: Consumer self-registers at provider marketplace.
    Given M&P Operations is registered as a participant in the data space.
    And Fancy Marketplace is registered as a participant in the data space.
    And M&P Operations allows self-registration of organizations.
    When Fancy Marketplace representative issues a user credential.
    And Fancy Marketplace representative registers at M&P Operations.
    Then Fancy Marketplace is registered as an organization at M&P Operations.

  Scenario: Consumer buys access to small offering and can create restricted clusters.
    Given M&P Operations is registered as a participant in the data space.
    And Fancy Marketplace is registered as a participant in the data space.
    And M&P Operations allows self-registration of organizations.
    And M&P Operations allows to buy its offerings.
    And M&P Operations allows operators to create clusters.
    When The provider creates a K8S Small product specification with credentials and policy config.
    And The provider creates a Small product offering referencing the Small specification.
    And Fancy Marketplace representative issues a user credential.
    And Fancy Marketplace operator issues an operator credential.
    And Fancy Marketplace representative registers at M&P Operations.
    And Fancy Marketplace representative buys the first available offering.
    Then Fancy Marketplace operator can create a K8S cluster with 3 nodes.
    And Fancy Marketplace operator cannot create a K8S cluster with 4 nodes.

  Scenario: Consumer buys access to full offering and can create unrestricted clusters.
    Given M&P Operations is registered as a participant in the data space.
    And Fancy Marketplace is registered as a participant in the data space.
    And M&P Operations allows self-registration of organizations.
    And M&P Operations allows to buy its offerings.
    And M&P Operations allows operators to create clusters.
    When The provider creates a K8S Full product specification with credentials and policy config.
    And The provider creates a Full product offering referencing the Full specification.
    And Fancy Marketplace representative issues a user credential.
    And Fancy Marketplace operator issues an operator credential.
    And Fancy Marketplace representative registers at M&P Operations.
    And Fancy Marketplace representative buys the first available offering.
    Then Fancy Marketplace operator can create a K8S cluster with 3 nodes.
    And Fancy Marketplace operator can create a K8S cluster with 5 nodes.
