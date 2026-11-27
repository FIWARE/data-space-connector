@dsp
Feature: One DCP participant context attached to two dataspace profiles.

  # Both DCP contexts carry http-dsp-profile-2025-1 and dsp-fiware-ds2 (k3s/dsp-provider.yaml,
  # k3s/dsp-consumer.yaml). fdsc-edc resolves context and profile before it checks the DCP token,
  # so routing answers are observable without credentials.

  Scenario Outline: Consumer reads the provider catalog through the profile <profile>.
    Given The consumer identity is properly setup.
    And The provider identity is properly setup.
    And The provider catalog is properly setup.
    When The consumer requests the provider catalog through the DSP profile "<profile>".
    Then The catalog read through the DSP profile contains at least one dataset.

    Examples:
      | profile                 |
      | http-dsp-profile-2025-1 |
      | dsp-fiware-ds2          |

  Scenario Outline: The provider routes a tokenless catalog request for the profile <profile> to <status>.
    When A tokenless catalog request is sent to the provider through the DSP profile "<profile>".
    Then The provider answers the tokenless catalog request with status <status>.

    Examples:
      | profile                 | status |
      | http-dsp-profile-2025-1 | 401    |
      | dsp-fiware-ds2          | 401    |
      | dsp-unknown             | 404    |

  Scenario: The provider context advertises exactly its two profiles.
    When The provider context's DSP version metadata is requested without a token.
    Then The DSP version metadata lists exactly the profiles "http-dsp-profile-2025-1,dsp-fiware-ds2".
