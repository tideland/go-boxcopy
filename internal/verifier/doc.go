package verifier

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

// Package verifier implements read-only comparison of source and target IMAP
// mail trees. It checks folder structure, per-folder message counts, and total
// per-folder RFC822 sizes to confirm that a migration completed correctly.
// No message bodies or headers are downloaded; no data is modified on either
// server.
