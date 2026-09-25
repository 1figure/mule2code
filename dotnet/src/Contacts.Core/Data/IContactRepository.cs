using Contacts.Core.Model;

namespace Contacts.Core.Data;

/// <summary>The two database operations of the Mule applications, on the <c>contacts</c> table.</summary>
public interface IContactRepository
{
    /// <summary>
    /// <c>&lt;db:bulk-insert doc:name="Contact Data"&gt;</c>: inserts all <paramref name="contacts"/> in one transaction,
    /// one <c>INSERT</c> per row. Any failure rolls the transaction back and throws.
    /// </summary>
    /// <exception cref="RepositoryException">The database rejected the insert; the driver error is the inner exception.</exception>
    Task BulkInsertAsync(IReadOnlyList<Contact> contacts, CancellationToken cancellationToken);

    /// <summary>
    /// <c>&lt;db:select doc:name="Select contacts by city"&gt;</c>: contacts whose <c>mailing_city</c> equals
    /// <paramref name="city"/> case-insensitively, ordered by last name, first name, contact id, at most <paramref name="limit"/>.
    /// </summary>
    /// <exception cref="RepositoryException">The query failed; the driver error is the inner exception.</exception>
    Task<IReadOnlyList<ContactRow>> QueryByCityAsync(string city, int limit, CancellationToken cancellationToken);
}

/// <summary>
/// A database operation failed (the Mule <c>DB:CONNECTIVITY</c> / <c>DB:QUERY_EXECUTION</c> errors);
/// the driver exception is the inner exception.
/// </summary>
public sealed class RepositoryException : Exception
{
    /// <summary>Creates the exception wrapping the driver error.</summary>
    public RepositoryException(string message, Exception inner) : base(message, inner)
    {
    }
}
