// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/forms/form_state.dart';
import 'package:plux_flutter/src/forms/validators.dart';
import 'package:plux_flutter/src/l10n/runtime_strings.dart';

/// Translatable form validator messages (I18N-001, ACT-020).
void main() {
  RuntimeStrings table(String locale) {
    final raw = jsonDecode(
      File('../../schema/l10n/plux/$locale.json').readAsStringSync(),
    ) as Map<String, Object?>;
    return RuntimeStrings({for (final e in raw.entries) e.key: '${e.value}'});
  }

  Map<String, Object?> errorsOf(FormTexts texts, {fbs.Value? ref}) {
    var state = <String, Object?>{};
    final decl = FormDecl(
      name: 'f',
      fields: [
        FormFieldDecl(
          name: 'name',
          type: null,
          initial: '',
          validators: [
            FieldValidator(
              kind: fbs.ValidatorKind.Required,
              messageRef: ref,
              check: (v) => v == null || v == ''
                  ? const ValidationFailure('required')
                  : null,
            ),
          ],
        ),
      ],
    );
    final host = FormHost(
      read: () => state.isEmpty ? null : state,
      write: (v) => state = v,
      evaluate: (_, _) => null,
      texts: texts,
    );
    FormController(decl, host).writeTouched('name', touched: true);
    return state['errors']! as Map<String, Object?>;
  }

  final ref = fbs.Value(
    fbs.ValueObjectBuilder(kind: fbs.ValueKind.Translation).toBytes(),
  );

  test('a referenced message shows in the effective locale [I18N-001]', () {
    final errors = errorsOf(
      FormTexts(reference: (_) => 'Bitte Namen angeben'),
      ref: ref,
    );
    expect(errors['name'], 'Bitte Namen angeben');
  });

  test('an unresolvable reference falls back to the built-in message '
      '[I18N-001]', () {
    final errors = errorsOf(FormTexts(reference: (_) => null), ref: ref);
    expect(errors['name'], 'Required');
  });

  test('the built-in messages come from the runtime strings [ACT-020]', () {
    final ar = table('ar');
    expect(builtInMessage(const ValidationFailure('required'), ar), 'مطلوب');
    expect(
      builtInMessage(const ValidationFailure('tooShort', {'min': 3}), ar),
      '3 أحرف على الأقل',
    );
    expect(errorsOf(FormTexts(strings: () => ar))['name'], 'مطلوب');
    expect(
      builtInMessage(const ValidationFailure('tooLong', {'max': 8})),
      'At most 8 characters',
    );
  });

  test('every locale translates every validation key [ACT-020]', () {
    for (final locale in ['de', 'ar', 'am']) {
      final t = table(locale);
      for (final key in englishRuntimeStrings.keys.where(
        (k) => k.startsWith('plux.validation.'),
      )) {
        expect(t.table.containsKey(key), isTrue, reason: '$locale $key');
      }
    }
  });
}
