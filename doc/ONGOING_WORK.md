# Ongoing Work

The FIWARE Data Space Connector is constantly beeing developed and extended with new features. Their status and some previews will be listed on this page.

All planned work is listed in the [FIWARE Data Space Connector Taiga-Board](https://tree.taiga.io/project/dwendland-fiware-data-space-connector/epics).

## Transfer Process Protocol

In order to be compatible with other Connectors, the FIWARE Data Space Connector will support the [IDSA Transfer Process Protocol](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/transfer-process/transfer.process.protocol). 
The architecture to be implemented is drafted in the following diagramm:

![TPP Draft](./img/tpp-draft.png)

* [Rainbow](https://github.com/ging/rainbow)(not yet public) is a RUST-implementation of the [Dataspace Protocol](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol), used for providing the  [Transfer Process API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/transfer-process/transfer.process.protocol), the [Catalog API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/catalog/catalog.protocol) and the Agreement-Part of the [Contract Negotiation API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/contract-negotiation/contract.negotiation.protocol)
* the [Contract Management](https://github.com/FIWARE/contract-management) component is beeing extended to:
    * integrate with the TMForum API and translate its entities to [DCAT Entries](https://www.w3.org/TR/vocab-dcat-3/) in Rainbow([Catalogs](https://www.w3.org/TR/vocab-dcat-3/#Class:Catalog) and [DataServices](https://www.w3.org/TR/vocab-dcat-3/#Class:Data_Service))
    * create Agreements in Rainbow based on the Product Orderings
    * create Policies at the [ODRL-PAP](https://github.com/wistefan/odrl-pap) based in the Product Orderings

The current state of work can be found at the [TPP-Integration Branch](https://github.com/FIWARE/contract-management/tree/tpp-integration) of the Contract Management. 
