# Mule Project `batch-contacts-csv-to-db` Overview

The Mule project `batch-contacts-csv-to-db` originated in October 2022 as a proof of concept for processing millions of records using Mule 4. The initial requirements included reading contact data from a CSV file and inserting the data into a database. The project has since evolved into a foundational example for understanding the core concepts of batch processing in Mule 4. Rather than addressing a specific business use case, the implementation is intentionally focused on illustrating Mule batch processing capabilities without introducing unnecessary integration complexity.

![Contacts CSV File to Database Introduction](assets/images/01-introduction.png)

As the diagram illustrates, the Mule application monitors the `new` subdirectory on an SFTP server for new or updated files. The application expects CSV files containing contact data. When a new or updated file is detected, the application reads the contact data and bulk inserts the records into a PostgreSQL database using a Batch Job component. Upon completion of the Batch Job, the application sends a summary report via Gmail.

## Design Considerations

This project is intentionally focused on demonstrating the fundamental concepts of batch processing in Mule 4. Some implementation choices have therefore been kept simple or retained to illustrate additional capabilities rather than represent recommended production implementations.

### Error Handling

The error-handling implementation is intentionally simple. It introduces record-level failure handling within a Batch Job and demonstrates how failed records can be processed separately. A production implementation would typically require additional error classification, recovery, observability, and operational considerations.

### Email Summary Report

The email summary report originated as a requirement for an earlier demonstration and remains to illustrate one approach to post-processing actions after a Batch Job completes. Email reporting is not an inherent requirement of Mule batch processing and can be removed or replaced with another reporting or notification mechanism.

### Record Validation

The main Batch Step includes a simple email-address validation operation to demonstrate record-level validation and failure handling.

## Implementation Overview

> [!NOTE]
> This section assumes familiarity with Anypoint Studio and Mule flows, as well as an imported copy of the Mule project. Consequently, the discussion focuses on the implementation rather than providing detailed instructions or XML snippets.

The Contacts CSV File to Database example is implemented as an integration and does not expose a REST endpoint. The implementation uses two Mule configuration (XML) files to separate global configuration from application logic.

- The file `global.xml` contains the global elements, centralizing configuration shared across Mule configuration files.
- The file `batch-contacts-csv-to-db-impl.xml` contains the application implementation.

![Mule configuration files](assets/images/02-mule-config-files.png)

The project uses YAML properties files with the naming convention `mule-props-<environment>` to support environment-specific configuration, such as `mule-props-dev.yaml`, `mule-props-test.yaml`, and `mule-props-staging.yaml`.

![Properties file template](assets/images/03-properties-files.png)

> [!IMPORTANT]
> In accordance with information security best practices, the repository does not include environment-specific properties files because they contain credentials. The provided template can be used to create properties files appropriate for the target environment and resources.

### Mule Configuration File `global.xml`

The `global.xml` file contains a `Global Property` element named `env`, which specifies the current environment, such as `dev` in the screen capture.

![env global property](assets/images/04-env-global-property.png)

> [!TIP]
> This approach allows the property to be overridden when deploying the application to other environments, such as test, staging, or production, resulting in the corresponding environment-specific properties file being loaded.

The `Configuration properties` element references a file that follows the `mule-props-<environment>` naming convention, enabling environment-specific configuration.

![Configuration properties](assets/images/05-configuration-properties.png)

### Mule Configuration File `batch-contacts-csv-to-db-impl.xml`

The Mule configuration file `batch-contacts-csv-to-db-impl.xml` contains the implementation of the Contacts CSV File to Database example.

![Implementation Overview](assets/images/06-implementation-overview.png)

As the screen capture illustrates, the implementation contains three Mule flows:

- `contacts-batch-process-flow` is the main flow.
- `initialization-flow` and `send-email-report-flow` are child flows used to separate supporting logic from the main flow.

#### Child Flow `initialization-flow`

The child flow `initialization-flow` handles initialization tasks, primarily setting variables used during processing.

![Child Flow `initialization-flow`](assets/images/07-child-flow-initialization-flow.png)

- The variable `startTime` captures the overall process start time for calculating the total processing duration.
- The variable `currentFilename` captures the source filename from the message attributes before those attributes are overridden by another processor.
- The variable `timestamp` captures the current timestamp for use in filenames, providing consistency and traceability.
- The variable `newFilename` contains the filename used when moving and renaming the processed file. It combines `currentFilename` and `timestamp` to indicate when the file was processed, for example, `contact-data-1m.20250506T224149099.csv`.
- The variable `errorsFilename` contains the filename used to record errors that occur during the Batch Job, for example, `contact-data-1m.20250506T224149099.errors.json`.

#### Child Flow `send-email-report-flow`

The child flow `send-email-report-flow` sends a summary report via Gmail upon completion of the Batch Job.

![Child Flow `send-email-report-flow`](assets/images/08-child-flow-send-email-report-flow.png)

As the screen capture illustrates, a [`Parse Template` processor](https://docs.mulesoft.com/mule-runtime/latest/parse-template-reference) generates the email body. The template is located in [`src/main/resources/parse-template`](../src/main/resources/parse-template/contacts-batch-report-email.template). The implementation uses Gmail to send the summary report, although the email configuration can be adapted to use another supported email service or server.

#### Main Flow `contacts-batch-process-flow`

The following screen capture provides a high-level overview of the main flow `contacts-batch-process-flow`.

![Main Flow `contacts-batch-process-flow`](assets/images/09-main-flow-contacts-batch-process-flow.png)

The `On New or Updated File` processor monitors the `new` subdirectory for new or updated files. If an error occurs outside the Batch Job during processing, the error handler moves the file to the `failed` subdirectory.

> [!NOTE]
> As described in the [Batch Processing documentation](https://docs.mulesoft.com/mule-runtime/latest/batch-processing-concept#error-handling), the Batch Job component handles record-level failures to prevent a single record failure from causing the entire batch job to fail. Consequently, the flow-level error handler handles errors that occur outside the Batch Job component, either before or after batch processing.

According to the [Batch Processing documentation](https://docs.mulesoft.com/mule-runtime/latest/batch-processing-concept#valid_input), the Batch Job component accepts Java `Iterable`, `Iterator`, and `Array` values, as well as JSON and XML payloads. Because CSV is not a valid Batch Job input, the `Transform Message CSV to Java` processor converts the CSV data before it enters the Batch Job.

> [!TIP]
> Mule runtime logging is intentionally limited by default. This implementation uses TRACE-level loggers extensively to provide visibility into flow and batch processing. These loggers are also useful during unit testing and debugging.

The following screen capture provides an overview of the Batch Job `file-processing-job`.

![Batch Job `file-processing-job`](assets/images/10-batch-job-file-processing-job.png)

The Batch Job contains two Batch Step components:

1. The Batch Step `main-processing-step` implements the load portion of the process by bulk inserting contact records into the database.
2. The Batch Step `failed-records-processing-step` handles records that were not processed successfully by the preceding step, such as records that fail validation or encounter an error during database insertion.

The following screen capture shows the configuration of the Batch Job `file-processing-job`.

![Batch Job Settings](assets/images/11-batch-job-settings.png)

- The `Max Failed Records` setting is configured as `-1`, indicating no limit, and the `Batch Block Size` setting is configured through a property. All other settings use their default values.

The following screen capture shows the configuration of the Batch Step `main-processing-step`.

![Batch Step `main-processing-step` Settings](assets/images/12-main-batch-step-settings.png)

- The `Accept Policy` setting uses the default value `NO_FAILURES`, which prevents records that previously failed from entering this step.

The following screen capture shows the processors included in the Batch Step `main-processing-step`.

![Batch Step `main-processing-step`](assets/images/13-main-batch-step.png)

- The `Processors` section intentionally contains only the `Validation Is email` operation to keep the example focused. Additional processors can be included as required. Processors within a Batch Step are applied to each record individually and can therefore affect overall processing time.

The following screen capture shows the configuration of the Batch Aggregator `main-records-aggregator`.

![Main Batch Aggregator Settings](assets/images/14-main-batch-aggregator-settings.png)

- The `Aggregator Size` setting is configured through a property, and `Preserve Mime Types` is enabled. The Batch Aggregator collects records according to the configured `Aggregator Size` and passes the resulting array to its processors. For example, with an aggregator size of 1,000, the `Transform Message CSV to SQL` and `Bulk Insert Contact Data` processors operate on 1,000 records at a time.

> [!NOTE]
> The CSV file and database table intentionally use the same data model. Consequently, the `Transform Message CSV to SQL` processor performs a straightforward DataWeave mapping without substantive data transformation.

The following screen capture shows the configuration of the Batch Step `failed-records-processing-step`.

![Batch Step `failed-records-processing-step` Settings](assets/images/15-failures-batch-step-settings.png)

- The `Accept Policy` setting uses the value `ONLY_FAILURES`, allowing the step to process only records that failed during preceding batch processing.

The following screen capture shows the processors included in the Batch Step `failed-records-processing-step`.

![Batch Step `failed-records-processing-step`](assets/images/16-failures-batch-step.png)

The failure-handling implementation is intentionally simple while demonstrating one approach to processing failed records:

- The `Transform Message Create Error Record` processor handles each failed record individually and creates a JSON object containing the error information and associated record to support troubleshooting.
- The Batch Aggregator `failed-records-aggregator` collects the error objects and writes them to a dedicated error file in the SFTP server's `processed` subdirectory alongside the processed source file. Alternative implementations could route the error information to another destination, such as an email attachment.

The following screen capture shows the `On Complete` section of the Batch Job component.

![Batch Job On Complete](assets/images/17-batch-job-on-complete.png)

The `On Complete` implementation performs two primary tasks:

- Logs the batch job report object, which provides statistics and additional information about the batch job execution.
- Sends a summary report via Gmail.

The following screen capture shows an example summary report.

![Summary Report Example](assets/images/18-summary-report-example.png)

---

Copyright © 2026 Alan Belisle. Licensed under the [Apache License 2.0](/LICENSE).
