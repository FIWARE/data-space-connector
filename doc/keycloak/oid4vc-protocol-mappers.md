# OID4VC Protocol Mappers — Keycloak 26.6.1

> **Feature flags required:** `keycloak.features.enabled` must contain both
> `oid4vc-vci` and (since KC 26.4+) `oid4vc-vci-preauth-code` for the
> pre-authorized_code grant to work.
>
> **`name` field:** A free-form identifier you choose to recognise the mapper
> within Keycloak. It has no functional impact — use any value that is
> meaningful to you (e.g. `"my-email-mapper"`, `"context-w3c"`, etc.). KC requires
> the `name` to be unique within a single ClientScope.

## Changes since 26.3.3

KC 26.4 redesigned the OID4VCI realm model
([keycloak/keycloak#39768](https://github.com/keycloak/keycloak/pull/39768)).
The mapper-level changes that affect the examples below:

- **`supportedCredentialTypes` is removed from every mapper config.**
  Mapper-to-credential association is now structural: each mapper belongs
  to exactly one ClientScope, and that ClientScope is the credential. To
  apply the same mapper logic to several VCs, declare the mapper once
  under each VC's `protocolMappers` list.
- **`subjectProperty` was renamed to `claim.name`** on every mapper that
  had it (`oid4vc-user-attribute-mapper`, `oid4vc-target-role-mapper`,
  `oid4vc-static-claim-mapper`, `oid4vc-generated-id-mapper`,
  `oid4vc-issued-at-time-claim-mapper`).
- **`oid4vc-subject-id-mapper`** uses `claim.name` plus `userAttribute`
  (the legacy `subjectIdProperty` is gone).
- **`oid4vc-vc-type-mapper` is deprecated.** The Verifiable Credential Type
  is now a ClientScope attribute (`vc.verifiable_credential_type`) instead of
  a per-credential mapper.

The mappers themselves still live in the realm under
`keycloak.realm.verifiableCredentials.<name>.protocolMappers`; the DSC
chart renders one ClientScope (with `protocol: "oid4vc"`) per VC entry.

---

## Table of Contents

- [oid4vc-context-mapper](#oid4vc-context-mapper)
- [oid4vc-vc-type-mapper (deprecated)](#oid4vc-vc-type-mapper-deprecated)
- [oid4vc-subject-id-mapper](#oid4vc-subject-id-mapper)
- [oid4vc-generated-id-mapper](#oid4vc-generated-id-mapper)
- [oid4vc-user-attribute-mapper](#oid4vc-user-attribute-mapper)
- [oid4vc-target-role-mapper](#oid4vc-target-role-mapper)
- [oid4vc-static-claim-mapper](#oid4vc-static-claim-mapper)
- [oid4vc-issued-at-time-claim-mapper](#oid4vc-issued-at-time-claim-mapper)

---

## oid4vc-context-mapper

**Name:** Credential Context Mapper

Assigns a `@context` entry to the Verifiable Credential. This is required by the W3C VC Data Model to define the semantic meaning of the credential's fields.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `context` | Verifiable Credentials Context | String | `https://www.w3.org/2018/credentials/v1` | The context URL to assign to the credential. |

### Example output

```json
{
  "@context": [
    "https://www.w3.org/2018/credentials/v1",
    "https://example.org/my-custom-context/v1"
  ]
}
```

### Usage example

```yaml
- name: context-mapper
  protocol: oid4vc
  protocolMapper: oid4vc-context-mapper
  config:
    context: https://www.w3.org/2018/credentials/v1
```

---

## oid4vc-vc-type-mapper (deprecated)

> :warning: **Deprecated in KC 26.4+.** The Verifiable Credential Type is now
> a ClientScope attribute (`vc.verifiable_credential_type`), not a per-mapper
> setting. The DSC chart populates it from
> `keycloak.realm.verifiableCredentials.<name>.attributes.verifiable_credential_type`
> and also auto-derives `vc.supported_credential_types` from it (controlling
> the `type` array of JWT-VC JSON credentials). Do not declare this mapper in
> new realms; the section is kept here only for users migrating from 26.3.

**Name:** Credential Type Mapper

(Legacy) Assigned an additional `type` entry to the Verifiable Credential. Use
the ClientScope attribute below instead.

### Replacement (10.x)

```yaml
keycloak:
  realm:
    verifiableCredentials:
      EmployeeCredential:
        attributes:
          verifiable_credential_type: "EmployeeCredential"
          # supported_credential_types is auto-derived from the line above;
          # set it explicitly only if you need a different value in the
          # JWT-VC JSON `type` array than the SD-JWT `vct`.
```

---

## oid4vc-subject-id-mapper

**Name:** CredentialSubject ID Mapper

Assigns an `id` to the `credentialSubject` based on a configured user attribute (e.g. the user's DID, username, email, or the internal Keycloak user ID).

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `claim.name` | Subject ID Property Name | String | `id` | The property name that will hold the subject ID inside `credentialSubject`. |
| `userAttribute` | User Attribute | List | `did` | Which user attribute supplies the value. Allowed: `did`, `username`, `email`, `id`. |

### Example output

```json
{
  "credentialSubject": {
    "id": "did:example:ebfeb1f712ebc6f1c276e12ec21"
  }
}
```

### Usage example

```yaml
- name: subject-id-mapper
  protocol: oid4vc
  protocolMapper: oid4vc-subject-id-mapper
  config:
    claim.name: id
    userAttribute: did
```

---

## oid4vc-generated-id-mapper

**Name:** Generated ID Mapper

Assigns a **randomly generated** ID to the credential subject. Unlike
`oid4vc-subject-id-mapper`, this always generates a new ID — useful when
you don't want to expose the user's actual identifier.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `claim.name` | ID Property Name | String | `id` | Name of the property to contain the generated id. |

### Example output

```json
{
  "credentialSubject": {
    "id": "urn:uuid:3d3c4b1a-9f2e-4c3b-8a1d-2f5e6b7c8d9e"
  }
}
```

### Usage example

```yaml
- name: generated-id-mapper
  protocol: oid4vc
  protocolMapper: oid4vc-generated-id-mapper
  config:
    claim.name: id
```

---

## oid4vc-user-attribute-mapper

**Name:** User Attribute Mapper

Maps a standard Keycloak user attribute directly into the `credentialSubject`. This is one of the most commonly used mappers to include user profile data in a credential.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `claim.name` | Attribute Property Name | String | — | Property to add the user attribute to in the credential subject. |
| `userAttribute` | User Attribute | List | — | The user attribute to be added to the credential subject. |
| `aggregateAttributes` | Aggregate Attributes | Boolean | `false` | Should the mapper aggregate user attributes. |

#### Available `userAttribute` options

- `username`
- `locale`
- `firstName`
- `lastName`
- `disabledReason`
- `email`
- `emailVerified`

### Example output

```json
{
  "credentialSubject": {
    "firstName": "Jane",
    "lastName": "Doe",
    "email": "jane.doe@example.com"
  }
}
```

### Usage example

> 💡 Create one mapper per attribute you want to include.

```yaml
- name: user-email-mapper
  protocol: oid4vc
  protocolMapper: oid4vc-user-attribute-mapper
  config:
    claim.name: email
    userAttribute: email
    aggregateAttributes: "false"
```

---

## oid4vc-target-role-mapper

**Name:** Target-Role Mapper

Maps the roles assigned to a user into the `credentialSubject`, using the
**client ID as the target context**. Useful for expressing authorization roles
within a credential.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `claim.name` | Roles Property Name | String | `roles` | The property name to use in the `credentialSubject` for the roles. |
| `clientId` | Client ID | String | — | Property to configure the client to get the roles from. |

### Example output

```json
{
  "credentialSubject": {
    "roles": [
      {
        "target": "did:web:my-client.example.org",
        "names": ["admin", "editor"]
      }
    ]
  }
}
```

### Usage example

```yaml
- name: target-role-mapper
  protocol: oid4vc
  protocolMapper: oid4vc-target-role-mapper
  config:
    claim.name: roles
    clientId: did:web:my-client.example.org
```

> :warning: KC 26.4+ does not allow two `oid4vc-target-role-mapper` mappers with
> the same `claim.name` in the same scope (the realm import will fail with
> `Duplicate key [credentialSubject, roles]`). Pick one target per VC, or use
> distinct claim names. Note that the FIWARE vcverifier and the chart's
> ODRL/OPA helpers look for `credentialSubject.roles[*].target` by default,
> so deviating from the `roles` claim name requires a matching policy update.

---

## oid4vc-static-claim-mapper

**Name:** Static Claim Mapper

Allows setting a **hardcoded / static value** on any property of the
`credentialSubject`. Useful for adding fixed metadata that doesn't vary per
user, such as a schema version, an issuer-side identifier or a fixed
membership type.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `claim.name` | Static Claim Property Name | String | — | The property name to set the static value on. |
| `staticValue` | Static Claim Value | String | — | Value to be set for the property. |

### Example output

```json
{
  "credentialSubject": {
    "schemaVersion": "1.0"
  }
}
```

### Usage example

```yaml
- name: static-claim-mapper
  protocol: oid4vc
  protocolMapper: oid4vc-static-claim-mapper
  config:
    claim.name: schemaVersion
    staticValue: "1.0"
```

---

## oid4vc-issued-at-time-claim-mapper

**Name:** Issuance Date Claim Mapper

Sets the **issuance date/time** claim in the credential subject. Supports
truncation to larger time units to prevent user correlation based on precise
timestamps — a useful privacy measure.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `claim.name` | Time Claim Name | String | `iat` | The property name for the issuance time claim. |
| `truncateToTimeUnit` | Truncate To Time Unit | List | — | Truncate time to the first second of the chosen unit. Such as to prevent correlation of credentials based on this time value. |
| `valueSource` | Source of Value | List | `COMPUTE` | Tells the protocol mapper where to get the information. |

#### `truncateToTimeUnit` options

| Value | Description |
|---|---|
| `MINUTES` | Truncate to the start of the current minute |
| `HOURS` | Truncate to the start of the current hour |
| `HALF_DAYS` | Truncate to the start of the current half-day |
| `DAYS` | Truncate to the start of the current day |
| `WEEKS` | Truncate to the start of the current week |
| `MONTHS` | Truncate to the start of the current month |
| `YEARS` | Truncate to the start of the current year |

#### `valueSource` options

| Value | Description |
|---|---|
| `COMPUTE` | *(default)* Keycloak computes the current time in seconds at issuance. |
| `VC` | Reads the time from the Verifiable Credential's own `issuanceDate` field. |

### Example output

```json
{
  "credentialSubject": {
    "iat": 1713916800
  }
}
```

### Usage example

```yaml
- name: issued-at-time-mapper
  protocol: oid4vc
  protocolMapper: oid4vc-issued-at-time-claim-mapper
  config:
    claim.name: iat
    valueSource: COMPUTE
    truncateToTimeUnit: HOURS
```

---

## Quick Reference

| Mapper ID | Purpose |
|---|---|
| `oid4vc-context-mapper` | Add `@context` URL(s) to the credential |
| `oid4vc-subject-id-mapper` | Set the subject `id` from a chosen user attribute |
| `oid4vc-generated-id-mapper` | Always generate a new random subject `id` |
| `oid4vc-user-attribute-mapper` | Map user profile attributes to the subject |
| `oid4vc-target-role-mapper` | Map client roles (per `clientId`) to the subject |
| `oid4vc-static-claim-mapper` | Set a hardcoded value on any subject property |
| `oid4vc-issued-at-time-claim-mapper` | Set the issuance timestamp (with optional truncation) |
| ~~`oid4vc-vc-type-mapper`~~ | *Deprecated in 26.4+* — use the scope attribute `vc.verifiable_credential_type` instead |
