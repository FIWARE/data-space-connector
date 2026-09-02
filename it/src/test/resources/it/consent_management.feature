@consent
Feature: Consent-gated access to personal data as described in CONSENT_MANAGEMENT.md.

  # The core consent story: a data subject publishes personal data at the provider, a consumer is
  # denied access until the subject grants consent, and access follows the consent from then on.
  #
  # Two gates guard the `mp-data-service-consent` route. OPA authorizes the read on the presented
  # credential and deliberately carries no consent refinement, so the consent-filter plugin is the
  # component that decides on consent: it asks the OwnerResolver whose data was returned and checks
  # that owner's consent at the consent-manager.
  #
  # The consent itself is recorded through the real give-consent API - participants are onboarded, the
  # agreement the notice is projected from is seeded, and the subject grants with its own credential.
  # Nothing is written into the consent-manager's database.

  Scenario: A consumer is denied personal data while no consent exists.
    Given The provider allows reading personal profiles at OPA.
    And The data subject published a personal profile it owns.
    When The consumer requests the personal profile.
    Then The consumer is denied access to the personal profile.

  Scenario: The data subject grants consent and the same request succeeds.
    Given The provider allows reading personal profiles at OPA.
    And The data subject published a personal profile it owns.
    And Both participants are onboarded at the consent-manager.
    And A signed agreement between the participants covers the personal profile.
    And The data subject is registered at the provider and has a PDI account.
    When The data subject grants consent for its own data.
    Then The consumer can read the personal profile.

  Scenario: Withdrawing the consent denies the access again.
    Given The provider allows reading personal profiles at OPA.
    And The data subject published a personal profile it owns.
    And Both participants are onboarded at the consent-manager.
    And A signed agreement between the participants covers the personal profile.
    And The data subject is registered at the provider and has a PDI account.
    And The data subject granted consent for its own data.
    When The data subject withdraws its consent.
    Then The consumer is denied access to the personal profile.
