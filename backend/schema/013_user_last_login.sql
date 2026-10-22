-- When each user last signed in, shown in user management. NULL until the
-- first sign-in.
ALTER TABLE misko.users ADD COLUMN last_login_at timestamptz;
