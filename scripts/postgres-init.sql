-- Local isolated development databases. Run as the bootstrap superuser.
SELECT 'CREATE ROLE pandora_owner LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pandora_owner') \gexec
SELECT 'CREATE ROLE pandora_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pandora_app') \gexec
SELECT 'CREATE DATABASE pandora OWNER pandora_owner'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'pandora') \gexec
SELECT 'CREATE DATABASE pandora_test OWNER pandora_owner'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'pandora_test') \gexec
REVOKE ALL ON DATABASE pandora FROM PUBLIC;
REVOKE ALL ON DATABASE pandora_test FROM PUBLIC;
GRANT CONNECT ON DATABASE pandora, pandora_test TO pandora_app;

