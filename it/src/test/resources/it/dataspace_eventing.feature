@eventing
Feature: Data space eventing as described in doc/EVENTING.md.

  # The trust anchor's contract-management publishes the removal of a participant from the TIR;
  # provider and consumer subscribe and act on it according to their removal mode. The deployment's
  # trust (registration Job grants, the allowDataSpaceEvents policy) is checked, never re-created.

  Background:
    Given The data space publisher delivers events to both participants.
    And M&P Operations offers a managed kubernetes.
    And M&P Operations allows self-registration of organizations.
    And M&P Operations allows to buy its offerings.
    And Fancy Marketplace issues a user credential to its employee.
    And Fancy Marketplace registers itself at M&P Operations.

  Scenario: With the default report mode a removal changes nothing
    Then the provider's registration Job wrote the membership grant for Fancy Marketplace.
    When Fancy Marketplace buys access to M&P's k8s services.
    Then the provider's TIL holds an order-scoped grant for Fancy Marketplace.
    When the operator removes Fancy Marketplace from the trust anchor.
    Then the provider reports the removal of Fancy Marketplace in report mode without changes.
    And the report lists the membership and the order grant Fancy Marketplace still holds.
    And the provider's TIL still holds the same grants for Fancy Marketplace.

  Scenario: In dataspace mode only the membership grant is removed
    When the provider removes participants in dataspace mode, keeping the defaults.
    And Fancy Marketplace buys access to M&P's k8s services.
    Then the provider's TIL holds an order-scoped grant for Fancy Marketplace.
    When the operator removes Fancy Marketplace from the trust anchor.
    Then the provider revokes the membership of Fancy Marketplace.
    And the provider's TIL no longer holds the membership grant of Fancy Marketplace, but still its order grant.

  Scenario: An identifier on keep is untouched
    Given Kept Member is a participant with a membership grant at the provider.
    When the provider removes participants in dataspace mode, keeping the defaults and did:web:kept-member.org.
    And the operator removes Kept Member from the trust anchor.
    Then the provider keeps Kept Member and its membership grant.

  # The order comes before the publisher goes down: without it the membership grant would be the consumer's only
  # grant, and the sweep would delete the whole issuer instead of reaching scenario 4's state.
  Scenario: With the push path blocked the reconciliation sweep reaches the same state
    When the provider removes participants in dataspace mode, keeping the defaults.
    And Fancy Marketplace buys access to M&P's k8s services.
    Then the provider's TIL holds an order-scoped grant for Fancy Marketplace.
    When the publisher is scaled down.
    And the operator removes Fancy Marketplace from the trust anchor while the publisher is down.
    And the provider's contract-management restarts.
    Then the provider's reconciliation sweep revokes the membership of Fancy Marketplace.
    And the report lists the order grant, but no membership grant, as still held by Fancy Marketplace.
    And the provider's TIL no longer holds the membership grant of Fancy Marketplace, but still its order grant.

  # The second data space is simulated by its grant alone; no second publisher is deployed.
  Scenario: An entry attributed to another data space survives
    When the provider removes participants in dataspace mode, keeping the defaults.
    And Fancy Marketplace buys access to M&P's k8s services.
    Then the provider's TIL holds an order-scoped grant for Fancy Marketplace.
    When Fancy Marketplace also holds a membership grant of another data space at the provider.
    And the operator removes Fancy Marketplace from the trust anchor.
    Then the provider revokes the membership of Fancy Marketplace.
    And the report lists the order grant and the other data space's grant, but no membership grant, as still held by Fancy Marketplace.
    And the provider's TIL no longer holds the membership grant of Fancy Marketplace, but still its order grant and the other data space's grant.
