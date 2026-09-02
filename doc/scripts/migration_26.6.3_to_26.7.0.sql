-- Migrate user_ver_credential
ALTER TABLE user_ver_credential
RENAME COLUMN credential_scope_name TO client_scope_id;

ALTER TABLE user_ver_credential
ALTER COLUMN client_scope_id TYPE VARCHAR(36);

ALTER TABLE user_ver_credential DROP CONSTRAINT UK_KKUWUVD67ONTGSUGOGM8UEWRE;
ALTER TABLE user_ver_credential ADD CONSTRAINT UK_KKUWUVD67ONTGSUGOGM8UEWRE UNIQUE (user_id, client_scope_id);

-- Migrate relationships between user_ver_credential and client_scope
UPDATE user_ver_credential uvc
SET client_scope_id = cs.id
FROM client_scope cs
WHERE uvc.client_scope_id = cs.name;

-- Migrate relationships between issued_ver_credential and user_ver_credential
UPDATE issued_ver_credential ivc
SET ver_credential_id = uvc.id
FROM user_ver_credential uvc
JOIN client_scope cs ON cs.id = uvc.client_scope_id
WHERE cs.name = ivc.ver_credential_id;

-- Migrate issued_ver_credential
ALTER TABLE issued_ver_credential
RENAME COLUMN credential_type TO ver_credential_id;

ALTER TABLE issued_ver_credential
ALTER COLUMN ver_credential_id TYPE VARCHAR(36);

ALTER TABLE issued_ver_credential ADD CONSTRAINT fk_issued_ver_credential_vc
FOREIGN KEY (ver_credential_id) REFERENCES user_ver_credential (id);

-- changeSet: 26.7.0-cluster-event update
ALTER TABLE cluster_event
DROP CONSTRAINT pk_cluster_event;

ALTER TABLE cluster_event
ADD CONSTRAINT pk_cluster_event
PRIMARY KEY (id, target_cluster);


-- Update changesets checksums
UPDATE databasechangelog
SET md5sum = NULL
WHERE id = '26.7.0-verifiable-credential' AND author = 'keycloak';

UPDATE databasechangelog
SET md5sum = NULL
WHERE id = '26.7.0-46204-issued-ver-credential-table' AND author = 'keycloak';

UPDATE databasechangelog
SET md5sum = NULL
WHERE id = '26.7.0-cluster-event' AND author = 'keycloak';