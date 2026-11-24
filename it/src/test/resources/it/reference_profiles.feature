@reference
Feature: The reference profiles (charts/*/examples) deploy a working data space.

  # The trust anchor, the provider with its marketplace and the consumer, deployed from the
  # reference profiles with their placeholders filled in (k3s/reference). Only what a real
  # deployment does as well: the participants are registered at the trust anchor, the consumer
  # issues a credential to its employee, who reads the provider's data through its gateway.

  Scenario: The trust anchor lists the participants it registered.
    Given The trust anchor registers the provider and the consumer of the reference deployment.
    Then The trust anchor's registry lists the provider and the consumer.

  Scenario: A consumer's employee reads the provider's data with a Verifiable Credential.
    Given The trust anchor registers the provider and the consumer of the reference deployment.
    When The provider allows every participant to read its energy reports.
    And The provider stores an energy report.
    And The consumer issues a user credential to its employee.
    Then The employee reads the energy report through the provider's data service.

  Scenario: The provider's public services are protected and served.
    Then The provider's data service rejects a request without a token.
    And The provider's TM Forum API offers token-based access and rejects a request without a token.
    And The provider's marketplace portal is served.
