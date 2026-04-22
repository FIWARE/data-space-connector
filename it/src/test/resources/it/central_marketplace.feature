@central
Feature: Central Marketplace integration as described in CENTRAL_MARKETPLACE.md.

  # This feature covers the complete central marketplace flow documented in CENTRAL_MARKETPLACE.md.
  # The central marketplace enables a provider to offer services through a centralized marketplace
  # operated by the consumer organization (fancy-marketplace.biz), with contract management
  # handling notification flows between the marketplace and the provider.
  #
  #
  # The flow:
  # 1. Prepare the marketplace with TMForum access policies
  # 2. Provider prepares for integration (contract management policy, credential, registration)
  # 3. Provider creates product offering on the marketplace
  # 4. Consumer buys access through the marketplace
  # 5. Contract management notification activates the service at the provider

  Scenario: Central marketplace is prepared with access policies.
    Given The central marketplace PAP endpoint is available.
    When The central marketplace registers self-registration policies for legal persons.
    And The central marketplace registers product ordering policies.
    And The central marketplace registers product offering creation policies.
    And The central marketplace registers product offering read policies.
    And The central marketplace registers product specification policies.
    Then The central marketplace has all required TMForum access policies.

  Scenario: Provider prepares for central marketplace integration.
    Given M&P Operations is registered as a participant in the data space.
    And The central marketplace PAP endpoint is available.
    When The central marketplace registers self-registration policies for legal persons.
    And The central marketplace registers product ordering policies.
    And The central marketplace registers product offering creation policies.
    And The central marketplace registers product offering read policies.
    And The central marketplace registers product specification policies.
    And The provider creates a contract management access policy.
    And The provider obtains a LegalPersonCredential with REPRESENTATIVE role.
    And The provider registers at the central marketplace with contract management configuration.
    Then The provider organization is registered at the central marketplace.

  Scenario: Provider creates product offering on central marketplace.
    Given M&P Operations is registered as a participant in the data space.
    And The central marketplace PAP endpoint is available.
    When The central marketplace registers self-registration policies for legal persons.
    And The central marketplace registers product ordering policies.
    And The central marketplace registers product offering creation policies.
    And The central marketplace registers product offering read policies.
    And The central marketplace registers product specification policies.
    And The provider creates a contract management access policy.
    And The provider obtains a LegalPersonCredential with REPRESENTATIVE role.
    And The provider registers at the central marketplace with contract management configuration.
    And The provider creates a product specification on the central marketplace.
    And The provider creates a product offering on the central marketplace.
    Then One product offering is available at the central marketplace.

  Scenario: Consumer buys access through central marketplace and gains service access.
    Given M&P Operations is registered as a participant in the data space.
    And Fancy Marketplace is registered as a participant in the data space.
    And The central marketplace PAP endpoint is available.
    When The central marketplace registers self-registration policies for legal persons.
    And The central marketplace registers product ordering policies.
    And The central marketplace registers product offering creation policies.
    And The central marketplace registers product offering read policies.
    And The central marketplace registers product specification policies.
    And The provider creates a contract management access policy.
    And The provider obtains a LegalPersonCredential with REPRESENTATIVE role.
    And The provider registers at the central marketplace with contract management configuration.
    And The provider creates a product specification on the central marketplace.
    And The provider creates a product offering on the central marketplace.
    And The consumer obtains a UserCredential in SD-JWT format.
    And The consumer obtains an OperatorCredential.
    And The consumer cannot yet get an access token for the OperatorCredential at the provider.
    And The consumer registers at the central marketplace.
    And The consumer lists offerings at the central marketplace.
    And The consumer creates and completes an order at the central marketplace.
    Then The consumer can get an access token for the OperatorCredential at the provider.
