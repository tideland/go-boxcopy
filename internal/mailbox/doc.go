package mailbox

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

// Package mailbox implements one-way copying for a single IMAP mailbox.
//
// A Mailbox manages IMAP connections to source and target servers,
// copies folder structure and messages in a single pass with optional
// throttling and progress logging.
