@eventing
Feature: Data space eventing as described in doc/EVENTING.md.

  # The trust anchor's contract-management publishes the removal of a participant from the TIR;
  # provider and consumer subscribe and act on it according to their removal mode. The deployment's
  # trust (registration Job grants, the allowDataSpaceEvents policy) is checked, never re-created.
  # Scenarios tagged @typed-scopes need a contract-management that sends
  # scopeType=dataspace-membership to the TIL (IMPLEMENTATION_PLAN.md, "Known blocker").

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
    And the provider's TIL still holds the same grants for Fancy Marketplace.

  @typed-scopes
  Scenario: The removal report lists what the removed participant still holds
    When Fancy Marketplace buys access to M&P's k8s services.
    Then the provider's TIL holds an order-scoped grant for Fancy Marketplace.
    When the operator removes Fancy Marketplace from the trust anchor.
    Then the provider reports the removal of Fancy Marketplace in report mode without changes.
    And the report lists the membership and the order grant Fancy Marketplace still holds.
    And the provider's TIL still holds the same grants for Fancy Marketplace.
