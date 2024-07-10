Feature: The Data Space should support a basic data exchange between registered participants.

  Scenario: A registered consumer can retrieve data from a registered data provider.
    Given M&P Operations is registered as a participant in the data space.
    And Fancy Marketplace is registered as a participant in the data space.
    When M&P Operations registers a policy to allow every participant access to its energy reports.
    And M&P Operations creates an energy report.
    And Fancy Marketplace issues a credential to its employee.
    Then Fancy Marketplace' employee can access the EnergyReport.