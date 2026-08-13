CREATE INDEX CONCURRENTLY lw_151_access_verification_code_latest ON verification_code (email, created_at DESC);
