# Getting Started with Mule Project `batch-contacts-csv-to-db`

## Prerequisites

As described in the [Overview](overview.md), the Mule project `batch-contacts-csv-to-db` monitors a directory on an SFTP server for new or updated files. When a file is detected, the application reads the contact data and bulk inserts the records into a database table. Upon completion of the Batch Job, the application sends a summary report via Gmail.

Running the Mule project with the existing implementation requires the following configuration information:

1. **SFTP Server Configuration**:
   - Hostname or IP address.
   - Port number.
   - Username.
   - Password.
   - Working directory containing the following subdirectories:
     - `new` — Monitored by the Mule application for new or updated files using the SFTP Connector.
     - `processed` — Contains files moved after successful processing and error files generated for records that fail during the Batch Job.
     - `failed` — Contains files moved by the error handler when an error or failure occurs outside the Batch Job.

2. **Database Configuration**:
   - Hostname or IP address.
   - Port number.
   - Username.
   - Password.
   - Database name.
   - When using a database other than PostgreSQL:
     - Database vendor or type, such as Oracle or Microsoft SQL Server.
     - JDBC driver.
     - JDBC URL.
     - JDBC driver class name.
     - Database instance or service name.

3. **Gmail Configuration**:
   - Gmail username.
   - Gmail app password. See the note below.
   - Gmail email address used as the sender, for example, `Max Mule <max.mule@gmail.com>`.
   - Primary email recipient (`To` field). The current implementation supports a single primary recipient.

> [!NOTE]
> As described in [Connect to Gmail with Email Connector Examples - Mule 4](https://docs.mulesoft.com/email-connector/latest/email-gmail), an [app password](https://support.google.com/accounts/answer/185833) must be configured for the Gmail account used by the Mule 4 Email Connector.

## Getting Started

1. Clone the repository and import the project `batch-contacts-csv-to-db` into Anypoint Studio.

   ```sh
   git clone https://github.com/abelisle-mulesoft/mule-4-batch-job-examples.git
   ```

2. For a `dev` environment, copy the properties template `src/main/resources/properties/mule-props.template.yaml` to `src/main/resources/properties/mule-props-dev.yaml`.

3. Edit the properties file `src/main/resources/properties/mule-props-dev.yaml`.

   At a minimum, configure the following properties for the target environment:

   - `env.name` — Environment name, for example, `dev`.

   - **SFTP Connector Configuration**:
     - `sftp.host` — SFTP server hostname or IP address.
     - `sftp.port` — SFTP server port number.
     - `sftp.username` — SFTP server username.
     - `sftp.password` — SFTP server password.
     - `sftp.working_dir` — Base path on the SFTP server containing the `new`, `failed`, and `processed` subdirectories.

   - **Database Connector Configuration**:
     - `postgres_db.host` — Database hostname or IP address.
     - `postgres_db.port` — Database port number, for example, `5432`.
     - `postgres_db.username` — Database username.
     - `postgres_db.password` — Database password.
     - `postgres_db.db_name` — Database name, for example, `postgres`.

   - **Email Connector Configuration**:
     - `gmail.server.username` — Gmail username.
     - `gmail.server.password` — Gmail app password.
     - `gmail.address.from` — Sender email address, which must match the Gmail address.
     - `gmail.address.to` — Recipient email address.

   The following properties can optionally be adjusted for the target environment or processing requirements:

   - **Batch Job Components Configuration**:
     - `batch.job.block_size` — Number of records included in each block when splitting the source data for processing.
     - `batch.aggregator.main.size` — Number of records included in each array processed by the main aggregator.

   - **SFTP Connector Configuration**:
     - `sftp.new_dir` — Subdirectory within `sftp.working_dir` monitored for new or updated files, for example, `new`.
     - `sftp.failed_dir` — Subdirectory within `sftp.working_dir` where the error handler moves files when an error or failure occurs, for example, `failed`.
     - `sftp.processed_dir` — Subdirectory within `sftp.working_dir` where successfully processed files are moved, for example, `processed`.

4. Compile and run the project in Anypoint Studio as a smoke test.

   The sample file [contact-data-100.csv](../../sample-data/contact-data-100.csv) can optionally be uploaded to the `new` subdirectory on the SFTP server to validate the configuration. Upon completion of the Batch Job, the Mule application sends a summary report via Gmail. Successful processing of the sample file and receipt of the summary report provide confirmation that the required configuration is functioning as expected.

---

Copyright © 2026 Alan Belisle. Licensed under the [Apache License 2.0](/LICENSE).
