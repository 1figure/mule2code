# Mule Project `batch-contacts-csv-to-db`

The Mule project `batch-contacts-csv-to-db` provides a foundational example of batch processing in Mule 4. The implementation is intended as a starting point for understanding core Mule batch processing concepts without introducing unnecessary integration complexity. The application uses the SFTP Connector to monitor a directory for new or updated CSV files containing contact data. When a file is detected, the application reads the contact data and uses a Batch Job component to bulk insert the records into a database. Upon completion of the Batch Job, the application sends a summary report via Gmail.

## Documentation

The **documentation** folder contains:

- An [Overview](documentation/overview.md) document, which provides additional details on the Mule application `batch-contacts-csv-to-db` and its implementation and is recommended as the starting point.
- A [Getting Started](documentation/getting-started.md) document, which lists the prerequisites and provides details on setting up and running the Mule application `batch-contacts-csv-to-db`.

## Supporting Resources

- The [sample-data](../sample-data/) folder contains fictitious contact datasets used for demonstration and testing.
- The [database](database/) folder contains the SQL script used to create the PostgreSQL contacts table.

---

Copyright © 2026 Alan Belisle. Licensed under the [Apache License 2.0](/LICENSE).
