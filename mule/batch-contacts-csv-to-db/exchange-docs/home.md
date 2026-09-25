# Batch Contacts CSV to Database

This Mule application demonstrates a batch processing implementation that reads contact data from CSV files and bulk inserts the records into a PostgreSQL database.

The application uses an SFTP server for file ingestion and processing, validates individual records within a Mule Batch Job, handles failed records separately, and sends a summary report via Gmail upon completion.

For detailed implementation information and setup instructions, refer to the documentation included with the project.
