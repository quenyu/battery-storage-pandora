-- Create the application and test databases.
SELECT 'CREATE ROLE pandora_owner LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pandora_owner') \gexec
SELECT 'CREATE ROLE pandora_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pandora_app') \gexec
SELECT 'CREATE DATABASE pandora_storage OWNER pandora_owner'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'pandora_storage') \gexec
SELECT 'CREATE DATABASE pandora_test OWNER pandora_owner'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'pandora_test') \gexec
REVOKE ALL ON DATABASE pandora_storage, pandora_test FROM PUBLIC;
GRANT CONNECT ON DATABASE pandora_storage, pandora_test TO pandora_app;
