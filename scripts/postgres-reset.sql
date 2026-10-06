-- Local development only: drop the Pandora databases so postgres-init.sql recreates them empty.
DROP DATABASE IF EXISTS pandora_storage WITH (FORCE);
DROP DATABASE IF EXISTS pandora_test WITH (FORCE);
