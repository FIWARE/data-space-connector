# OID4VC Protocol Mappers — Keycloak 26.3.3

> **Feature flag required:** `KC_FEATURES=oid4vc-vci`
>
> **`name` field:** A free-form identifier you choose to recognize the mapper within Keycloak. It has no functional impact — use any value that is meaningful to you (e.g. `"my-email-mapper"`, `"context-w3c"`, etc.).
>
> All mappers share a common property: `supportedCredentialTypes` — a comma-separated list of credential types the mapper applies to. Defaults to `VerifiableCredential`.

---

## Table of Contents

- [oid4vc-context-mapper](#oid4vc-context-mapper)
- [oid4vc-vc-type-mapper](#oid4vc-vc-type-mapper)
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
| `supportedCredentialTypes` | Supported Credential Types | String | `VerifiableCredential` | Comma-separated list of credential types this mapper applies to. |
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

```json
{
  "name": "context-mapper",
  "protocol": "oid4vc",
  "protocolMapper": "oid4vc-context-mapper",
  "config": {
    "supportedCredentialTypes": "VerifiableCredential",
    "context": "https://www.w3.org/2018/credentials/v1"
  }
}
```

---

## oid4vc-vc-type-mapper

**Name:** Credential Type Mapper

Assigns an additional `type` entry to the Verifiable Credential. Use this to declare specific credential types beyond the base `VerifiableCredential`.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `supportedCredentialTypes` | Supported Credential Types | String | `VerifiableCredential` | Comma-separated list of credential types this mapper applies to. |
| `vcTypeProperty` | Verifiable Credential Type | String | — | The type value to add to the credential's `type` array. |

### Example output

```json
{
  "type": ["VerifiableCredential", "EmployeeCredential"]
}
```

### Usage example

```json
{
  "name": "vc-type-mapper",
  "protocol": "oid4vc",
  "protocolMapper": "oid4vc-vc-type-mapper",
  "config": {
    "supportedCredentialTypes": "VerifiableCredential",
    "vcTypeProperty": "EmployeeCredential"
  }
}
```

---

## oid4vc-subject-id-mapper

**Name:** CredentialSubject ID Mapper

Assigns an `id` to the `credentialSubject`. If no specific value is configured, a randomly generated ID is used.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `supportedCredentialTypes` | Supported Credential Types | String | `VerifiableCredential` | Comma-separated list of credential types this mapper applies to. |
| `subjectIdProperty` | ID Property Name | String | `id` | The property name that will hold the subject ID. |

### Example output

```json
{
  "credentialSubject": {
    "id": "did:example:ebfeb1f712ebc6f1c276e12ec21"
  }
}
```

### Usage example

```json
{
  "name": "subject-id-mapper",
  "protocol": "oid4vc",
  "protocolMapper": "oid4vc-subject-id-mapper",
  "config": {
    "supportedCredentialTypes": "VerifiableCredential",
    "subjectIdProperty": "id"
  }
}
```

---

## oid4vc-generated-id-mapper

**Name:** Generated ID Mapper

Assigns a **randomly generated** ID to the credential subject. Unlike `oid4vc-subject-id-mapper`, this always generates a new ID — useful when you don't want to expose the user's actual identifier.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `supportedCredentialTypes` | Supported Credential Types | String | `VerifiableCredential` | Comma-separated list of credential types this mapper applies to. |
| `subjectProperty` | ID Property Name | String | `id` | The property name that will hold the generated ID. |

### Example output

```json
{
  "credentialSubject": {
    "id": "urn:uuid:3d3c4b1a-9f2e-4c3b-8a1d-2f5e6b7c8d9e"
  }
}
```

### Usage example

```json
{
  "name": "generated-id-mapper",
  "protocol": "oid4vc",
  "protocolMapper": "oid4vc-generated-id-mapper",
  "config": {
    "supportedCredentialTypes": "VerifiableCredential",
    "subjectProperty": "id"
  }
}
```

---

## oid4vc-user-attribute-mapper

**Name:** User Attribute Mapper

Maps a standard Keycloak user attribute directly into the `credentialSubject`. This is one of the most commonly used mappers to include user profile data in a credential.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `supportedCredentialTypes` | Supported Credential Types | String | `VerifiableCredential` | Comma-separated list of credential types this mapper applies to. |
| `subjectProperty` | Attribute Property Name | String | — | The property name to use in the `credentialSubject` for this attribute. |
| `userAttribute` | User Attribute | List | — | The user attribute to include. See options below. |
| `aggregateAttributes` | Aggregate Attributes | Boolean | — | Whether to aggregate multi-valued user attributes. |

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

```json
{
  "name": "user-email-mapper",
  "protocol": "oid4vc",
  "protocolMapper": "oid4vc-user-attribute-mapper",
  "config": {
    "supportedCredentialTypes": "VerifiableCredential",
    "subjectProperty": "email",
    "userAttribute": "email",
    "aggregateAttributes": "false"
  }
}
```

---

## oid4vc-target-role-mapper

**Name:** Target-Role Mapper

Maps the roles assigned to a user into the `credentialSubject`, using the **client ID as the target context**. Useful for expressing authorization roles within a credential.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `supportedCredentialTypes` | Supported Credential Types | String | `VerifiableCredential` | Comma-separated list of credential types this mapper applies to. |
| `subjectProperty` | Roles Property Name | String | `roles` | The property name to use in the `credentialSubject` for the roles. |

### Example output

```json
{
  "credentialSubject": {
    "roles": [
      {
        "target": "my-client-id",
        "roles": ["admin", "editor"]
      }
    ]
  }
}
```

### Usage example

```json
{
  "name": "target-role-mapper",
  "protocol": "oid4vc",
  "protocolMapper": "oid4vc-target-role-mapper",
  "config": {
    "supportedCredentialTypes": "VerifiableCredential",
    "subjectProperty": "roles"
  }
}
```

---

## oid4vc-static-claim-mapper

**Name:** Static Claim Mapper

Allows setting a **hardcoded / static value** on any property of the `credentialSubject`. Useful for adding fixed metadata that doesn't vary per user, such as a schema version or issuer identifier.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `supportedCredentialTypes` | Supported Credential Types | String | `VerifiableCredential` | Comma-separated list of credential types this mapper applies to. |
| `subjectProperty` | Static Claim Property Name | String | — | The property name to set the static value on. |
| `staticValue` | Static Claim Value | String | — | The static value to assign to the property. |

### Example output

```json
{
  "credentialSubject": {
    "schemaVersion": "1.0"
  }
}
```

### Usage example

```json
{
  "name": "static-claim-mapper",
  "protocol": "oid4vc",
  "protocolMapper": "oid4vc-static-claim-mapper",
  "config": {
    "supportedCredentialTypes": "VerifiableCredential",
    "subjectProperty": "schemaVersion",
    "staticValue": "1.0"
  }
}
```

---

## oid4vc-issued-at-time-claim-mapper

**Name:** Issuance Date Claim Mapper

Sets the **issuance date/time** claim in the credential subject. Supports truncation to larger time units to prevent user correlation based on precise timestamps — a useful privacy measure.

### Properties

| Property | Label | Type | Default | Description |
|---|---|---|---|---|
| `supportedCredentialTypes` | Supported Credential Types | String | `VerifiableCredential` | Comma-separated list of credential types this mapper applies to. |
| `subjectProperty` | Time Claim Name | String | `iat` | The property name for the issuance time claim. |
| `truncateToTimeUnit` | Truncate To Time Unit | List | — | Truncates the timestamp to prevent correlation. See options below. |
| `valueSource` | Source of Value | List | `COMPUTE` | Where to get the time value from. See options below. |

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

```json
{
  "name": "issued-at-time-mapper",
  "protocol": "oid4vc",
  "protocolMapper": "oid4vc-issued-at-time-claim-mapper",
  "config": {
    "supportedCredentialTypes": "VerifiableCredential",
    "subjectProperty": "iat",
    "valueSource": "COMPUTE",
    "truncateToTimeUnit": "HOURS"
  }
}
```

---

## Quick Reference

| Mapper ID | Purpose |
|---|---|
| `oid4vc-context-mapper` | Add `@context` URL(s) to the credential |
| `oid4vc-vc-type-mapper` | Add a custom `type` to the credential |
| `oid4vc-subject-id-mapper` | Set the subject `id` (fixed or random) |
| `oid4vc-generated-id-mapper` | Always generate a new random subject `id` |
| `oid4vc-user-attribute-mapper` | Map user profile attributes to the subject |
| `oid4vc-target-role-mapper` | Map client roles to the subject |
| `oid4vc-static-claim-mapper` | Set a hardcoded value on any subject property |
| `oid4vc-issued-at-time-claim-mapper` | Set the issuance timestamp (with optional truncation) |