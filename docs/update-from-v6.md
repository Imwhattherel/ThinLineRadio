# Updating from Version 6 to Version 7

ThinLine Radio v7 has significant changes from version 6. This guide will help you migrate your configuration and understand what has changed.

> **REMEMBER TO ALWAYS BACKUP YOUR DATABASE AND CONFIGURATION BEFORE ATTEMPTING AN UPDATE.**

## Important Changes in Version 7

### Database Changes

- **SQLite is no longer supported**. ThinLine Radio v7 requires PostgreSQL.
- **Users will be dropped**. Version 7 introduces a completely new user system with email-based authentication, passwords, and password reset functionality. All existing users from version 6 will need to be recreated.

### Configuration Migration

Version 7 uses a new configuration format. You must export your v6 configuration and import it into v7.

## Update Steps

### 1. Export Your Version 6 Configuration

Before upgrading, export your configuration from your v6 instance:

1. Log into the admin panel of your v6 instance
2. Navigate to **Tools** → **Import/Export Config**
3. Click **Export Config** to download your configuration file
4. Save this file in a safe location

### 2. Install ThinLine Radio v7

1. Download the latest ThinLine Radio v7 release from the [releases tab](https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio/releases)
2. Extract the archive to a new folder (do not overwrite your v6 installation)
3. If you're using PostgreSQL (recommended), ensure PostgreSQL is installed and running
4. Create a new database for ThinLine Radio v7

### 3. Configure Database Connection

Start ThinLine Radio v7 with the appropriate database connection arguments:

```bash
./thinline-radio \
    -db_type postgresql \
    -db_host localhost \
    -db_port 5432 \
    -db_name thinline_radio \
    -db_username your_username \
    -db_password your_password
```

The server will automatically create the necessary database schema on first run.

### 4. Import Your Version 6 Configuration

1. Log into the admin panel of your v7 instance (default password: `admin`)
2. Navigate to **Tools** → **Import/Export Config**
3. Click **Import for Review** or **Import and Apply**
4. Select the configuration file you exported from v6
5. Review the imported configuration (especially systems, talkgroups, and groups)
6. Click **Save** to apply the configuration

The import process will automatically convert v6 configuration format to v7 format, including:
- Converting `_id` fields to `id` fields
- Converting `apiKeys` to `apikeys`
- Converting `dirWatch` to `dirwatch`
- Updating system, talkgroup, and unit references
- Converting single `groupId` to `groupIds` arrays

### 5. Review Your Configuration

After importing, review the following sections in the admin panel:

- **Systems**: Verify all systems are present and configured correctly
- **Talkgroups**: Check that talkgroups are properly assigned to systems
- **Groups**: Ensure groups are configured as expected
- **Tags**: Verify tag assignments
- **API Keys**: Check that API keys are present (you may need to regenerate some)
- **Dirwatch**: Review directory watch configurations
- **Downstreams**: Verify downstream configurations

### 6. Recreate Users

Since the user system has been completely rewritten:

1. Navigate to **Users** in the admin panel
2. Create new user accounts with email addresses
3. Set passwords for each user
4. Configure user groups and permissions as needed
5. Users will need to use email/password to log in (the old access code system is no longer used)

### 7. Configure as a Service

Once everything is working correctly, configure ThinLine Radio v7 to run as a service. See the platform-specific documentation:
- [Linux Installation](platforms/linux.md)
- [macOS Installation](platforms/macos.md)
- [Windows Installation](platforms/windows.md)
- [FreeBSD Installation](platforms/freebsd.md)

## Database Migration Notes

### PostgreSQL (Recommended)

PostgreSQL is the recommended database for ThinLine Radio v7. All features have been tested and verified with PostgreSQL.

**Installation:**
- Ubuntu/Debian: `sudo apt-get install postgresql postgresql-contrib`
- CentOS/RHEL: `sudo yum install postgresql-server postgresql-contrib`
- macOS: `brew install postgresql`

**Create Database:**
```bash
sudo -u postgres psql
CREATE DATABASE thinline_radio;
CREATE USER thinline_user WITH PASSWORD 'your_password';
GRANT ALL PRIVILEGES ON DATABASE thinline_radio TO thinline_user;
\q
```


## Troubleshooting

### Import Issues

If you encounter issues importing your v6 configuration:

1. Check the browser console for error messages
2. Verify the configuration file is valid JSON
3. Try importing sections individually if possible
4. Review the conversion notes in the import component code

### Database Connection Issues

If you have trouble connecting to the database:

1. Verify the database server is running
2. Check that the database name, username, and password are correct
3. Ensure the database user has the necessary permissions
4. Check firewall settings if connecting to a remote database

### User Authentication Issues

If users cannot log in:

1. Verify users have been created in the new system
2. Check that email addresses are correct
3. Use the password reset feature if needed
4. Ensure the email system is configured if using email verification

## Additional Resources

- [ThinLine Radio Repository](https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio)
- [ThinLine Radio Discussions](https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio/discussions)
- [ThinLine Radio Issues](https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio/issues)
