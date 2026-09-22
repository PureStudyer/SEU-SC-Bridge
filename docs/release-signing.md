# Signed release setup

The release workflow in `.github/workflows/release.yml` publishes a GitHub release when a `v*` tag is pushed. The current release mode is unsigned so it can be published without external certificates. Windows SmartScreen and macOS Gatekeeper may warn users; publish SHA-256 checksums and distribute only through a trusted channel until certificates are available.

## Apple Developer ID and notarization

1. Enroll in the [Apple Developer Program](https://developer.apple.com/programs/) as the account holder or an administrator. A free Apple account can test locally, but it cannot create the Developer ID certificate required for public macOS distribution. The program is normally USD 99/year; eligible nonprofits, accredited educational institutions, and government entities can request a fee waiver.
2. In **Certificates, Identifiers & Profiles → Certificates**, create a **Developer ID Application** certificate using the CSR prepared on the build Mac:

   ```text
   /Users/ankuntian/Library/Application Support/SEU SC Bridge/signing/apple-developer-id.csr
   ```

   Keep the matching private key on that Mac:

   ```text
   /Users/ankuntian/Library/Application Support/SEU SC Bridge/signing/apple-developer-id.key
   ```

   Never commit or send the private key. The CSR can be uploaded to Apple; the private key must stay with it.
3. Download the issued `.cer`, import it into Keychain Access on the build Mac, and export the certificate with its private key as a password-protected `.p12`. Alternatively, convert the downloaded DER certificate first and export with OpenSSL:

   ```bash
   openssl x509 -inform der -in DeveloperIDApplication.cer -out developer-id-application.pem
   openssl pkcs12 -export \
     -inkey "$HOME/Library/Application Support/SEU SC Bridge/signing/apple-developer-id.key" \
     -in developer-id-application.pem \
     -out developer-id-application.p12
   ```

4. In **App Store Connect → Users and Access → Integrations → App Store Connect API**, create a **team** API key. Download its `.p8` private key once, and record its Key ID and Issuer ID. The private key cannot be downloaded again.
5. Add these GitHub Actions secrets under **Repository Settings → Secrets and variables → Actions**:

   | Secret | Value |
   |---|---|
   | `APPLE_CERTIFICATE_P12_BASE64` | `base64 -i developer-id-application.p12 \| tr -d '\\n'` |
   | `APPLE_CERTIFICATE_PASSWORD` | Password used for the `.p12` export |
   | `APPLE_NOTARY_KEY_BASE64` | `base64 -i AuthKey_<KEY_ID>.p8 \| tr -d '\\n'` |
   | `APPLE_NOTARY_KEY_ID` | API key ID |
   | `APPLE_NOTARY_ISSUER` | API issuer ID |

The workflow imports the Developer ID certificate, signs with hardened runtime, submits each DMG to Apple notary service, staples the ticket, and verifies both the signature and notarization.

## Windows Authenticode

The free route for this MIT open-source project is to apply to [SignPath Foundation](https://signpath.org/apply.html). SignPath reviews the repository and, if accepted, signs release artifacts from the verified GitHub source. It requires an active public project, an OSI-approved license, documented functionality, a code-signing policy, and manual approval for releases; acceptance is not automatic. The official GitHub Action is documented by [SignPath](https://docs.signpath.io/trusted-build-systems/github).

For the SignPath route, add these GitHub Actions values after the project is approved and configured:

| Name | Kind | Value |
|---|---|---|
| `SIGNPATH_API_TOKEN` | Secret | SignPath CI API token |
| `SIGNPATH_ORGANIZATION_ID` | Repository variable | SignPath organization ID |
| `SIGNPATH_PROJECT_SLUG` | Repository variable | SignPath project slug |
| `SIGNPATH_SIGNING_POLICY_SLUG` | Repository variable | Usually `release-signing` |

The signed workflow variant can submit the Windows executable and installer to SignPath for signing. The current unsigned release workflow does not require these values.

If SignPath does not accept the project, obtain a publicly trusted Authenticode code-signing certificate from a commercial CA, or use Microsoft Artifact Signing. The alternative PFX route accepts a password-protected `.pfx` that includes the private key. Do not use a self-signed certificate for the formal release.

Add these GitHub Actions secrets:

| Secret | Value |
|---|---|
| `WINDOWS_SIGNING_CERTIFICATE_BASE64` | Base64 of the `.pfx` file |
| `WINDOWS_SIGNING_CERTIFICATE_PASSWORD` | Password protecting the `.pfx` |
| `WINDOWS_TIMESTAMP_URL` | Optional RFC 3161 endpoint; default is `http://timestamp.digicert.com` |

The workflow signs both the executable and NSIS installer with SHA-256 and an RFC 3161 timestamp, then verifies the installer with `signtool verify /pa`.

If Microsoft Artifact Signing is selected instead of a PFX certificate, the workflow needs to be switched to Microsoft's signing action and its Azure identity configuration; the PFX secrets above are not used for that service.

## Publish

After all secrets are present, push the release tag:

```bash
git tag v1.0.1
git push origin v1.0.1
```

GitHub Actions builds Windows `x64` and `arm64` installers plus macOS Intel and Apple Silicon DMGs, creates `SHA256SUMS.txt`, and publishes the release. The workflow can also be started manually from the Actions page with a tag such as `v1.0.1`.

The Apple Developer Program is required for Developer ID distribution and notarization. Apple API keys and their private `.p8` files are sensitive credentials; do not paste them into chat, issues, logs, or the repository.
