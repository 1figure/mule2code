using System.Text.RegularExpressions;

namespace Contacts.Core.Validation;

/// <summary>The Validation module's <c>is-email</c> operation as a static function.</summary>
public static partial class EmailValidation
{
    /// <summary>The <c>message</c> attribute of the <c>&lt;validation:is-email&gt;</c> element; ends up in the errors file.</summary>
    public const string InvalidEmailMessage = "Missing or invalid email";

    // Same shape the Mule Validation module accepts: local part, one '@', dotted domain with a 2+ letter TLD.
    [GeneratedRegex(@"^[A-Za-z0-9!#$%&'*+/=?^_`{|}~.-]+@[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*\.[A-Za-z]{2,}$")]
    private static partial Regex EmailPattern();

    /// <summary>Returns whether <paramref name="email"/> is a syntactically valid e-mail address.</summary>
    public static bool IsEmail(string? email)
    {
        if (string.IsNullOrWhiteSpace(email) || email.Length > 254)
        {
            return false;
        }

        var at = email.IndexOf('@', StringComparison.Ordinal);
        if (at <= 0 || at > 64 || email.StartsWith('.') || email[at - 1] == '.' || email.Contains("..", StringComparison.Ordinal))
        {
            return false;
        }

        return EmailPattern().IsMatch(email);
    }

    /// <summary>
    /// <c>&lt;validation:is-email doc:name="Is email" email="#[payload.email]" message="Missing or invalid email"&gt;</c>,
    /// the per-record processor of <c>main-processing-step</c>.
    /// </summary>
    /// <exception cref="InvalidEmailException">The row's <c>email</c> cell is missing or not an address.</exception>
    public static void ValidateEmail(IReadOnlyDictionary<string, string> row)
    {
        ArgumentNullException.ThrowIfNull(row);
        row.TryGetValue("email", out var email);
        if (!IsEmail(email))
        {
            throw new InvalidEmailException(InvalidEmailMessage);
        }
    }
}

/// <summary>Raised by <see cref="EmailValidation.ValidateEmail"/>; the Mule error type was <c>VALIDATION:INVALID_EMAIL</c>.</summary>
public sealed class InvalidEmailException : Exception
{
    /// <summary>Creates the exception with the validation message.</summary>
    public InvalidEmailException(string message) : base(message)
    {
    }
}
