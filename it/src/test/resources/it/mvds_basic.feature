Feature: The Data Space should support a basic data exchange between registered participants.

  Scenario: A registered consumer can retrieve data from a registered data provider.
    Given M&P Operations is registered as a participant in the data space.
    And Fancy Marketplace is registered as a participant in the data space.
    When M&P Operations registers a policy to allow every participant access to its energy reports.
    And M&P Operations creates an energy report.
    And Fancy Marketplace issues a user credential to its employee.
    Then Fancy Marketplace' employee can access the EnergyReport.

  Scenario: A registered operator can create a k8s cluster.
    Given M&P Operations is registered as a participant in the data space.
    And  M&P Operations offers a managed kubernetes.
    And M&P Operations allows self-registration of organizations.
    And M&P Operations allows to buy its offerings.
    And M&P Operations allows operators to create clusters.
    And Fancy Marketplace is registered as a participant in the data space.
    And Fancy Marketplace issues an operator credential to its employee.
    And Fancy Marketplace issues a user credential to its employee.
    And Fancy Marketplace is not allowed to create a cluster at M&P Operations.
    When Fancy Marketplace registers itself at M&P Operations.
    And Fancy Marketplace buys access to M&P's k8s services.
    Then Fancy Marketplace operators can create clusters.
    
  Scenario: A registered operator can access reports through the Transfer Process Protocol.
    Given M&P Operations is registered as a participant in the data space.
    And M&P Operations offers detailed reports.
    And M&P Operations allows to read its dcat-catalog.
    And M&P Operations allows self-registration of organizations.
    And M&P Operations allows to buy its offerings.
    And M&P Operations allows operators to read uptime reports.
    And M&P Operations allows operators to request data transfers.
    And M&P Operations allows to read its agreements.
    And Fancy Marketplace issues an operator credential to its employee.
    And Fancy Marketplace issues a user credential to its employee.
    And Fancy Marketplace is registered as a participant in the data space.
    Then M&P Operations uptime report service is offered at the IDSA Catalog Endpoint.
    When Fancy Marketplace registers itself at M&P Operations.
    And M&P Operations creates an uptime report.
    And Fancy Marketplace buys access to M&P's uptime reports.
    And Fancy Marketplace requests and starts the Data Transfer.
    Then Fancy Marketplace operators can get the report.