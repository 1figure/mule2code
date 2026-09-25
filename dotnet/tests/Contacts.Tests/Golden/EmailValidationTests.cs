using Contacts.Core.Validation;
using Contacts.Tests.Support;

namespace Contacts.Tests.Golden;

/// <summary><c>&lt;validation:is-email&gt;</c> with the fixture's addresses and a few invalid samples.</summary>
public sealed class EmailValidationTests
{
    [Fact]
    public void EveryEmailInContactData100IsValid()
    {
        var rows = TestData.ReadRows("contact-data-100.csv");

        Assert.All(rows, row => Assert.True(EmailValidation.IsEmail(row["email"]), row["email"]));
    }

    [Fact]
    public void ExactlyRows3And7OfTheWithErrorsFileAreInvalid()
    {
        var rows = TestData.ReadRows("contact-data-100-with-errors.csv");

        var invalid = rows.Where(row => !EmailValidation.IsEmail(row["email"])).Select(row => row["external_id"]).ToList();

        Assert.Equal(["3", "7"], invalid);
    }

    [Theory]
    [InlineData("")]
    [InlineData("   ")]
    [InlineData("not-an-email")]
    [InlineData("a@b")]
    [InlineData("@example.com")]
    [InlineData("user@")]
    [InlineData("user@.com")]
    [InlineData("user@example..com")]
    [InlineData("us er@example.com")]
    [InlineData(".user@example.com")]
    public void RejectsInvalidSamples(string email) => Assert.False(EmailValidation.IsEmail(email));

    [Fact]
    public void RejectsNull() => Assert.False(EmailValidation.IsEmail(null));

    [Theory]
    [InlineData("ypunton1y9w@flavors.me")]
    [InlineData("first.last+tag@sub.example.co.uk")]
    public void AcceptsValidSamples(string email) => Assert.True(EmailValidation.IsEmail(email));

    [Fact]
    public void ValidateEmailThrowsTheMessageFromTheXml()
    {
        var row = new Dictionary<string, string> { ["email"] = "not-an-email" };

        var e = Assert.Throws<InvalidEmailException>(() => EmailValidation.ValidateEmail(row));

        Assert.Equal("Missing or invalid email", e.Message);
    }

    [Fact]
    public void ValidateEmailTreatsAMissingCellAsInvalid() =>
        Assert.Throws<InvalidEmailException>(() => EmailValidation.ValidateEmail(new Dictionary<string, string>()));

    [Fact]
    public void ValidateEmailPassesAValidRow() =>
        EmailValidation.ValidateEmail(new Dictionary<string, string> { ["email"] = "a@example.com" });
}
