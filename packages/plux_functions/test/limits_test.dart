// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_functions/plux_functions.dart';

import 'support/support.dart';
import 'support/test_host.dart';

void main() {
  late FunctionEngine engine;
  late FunctionModule probe;
  late Directory scratch;
  late TestHost host;

  setUpAll(() async {
    engine = await FunctionEngine.start();
    probe = await engine.load(key: 'probe', bytes: fixture('probe.wasm'));
    scratch = Directory.systemTemp.createTempSync('plux_functions_limits');
    host = TestHost(directory: scratch.path);
  });

  tearDownAll(() async {
    await engine.close();
    scratch.deleteSync(recursive: true);
  });

  Future<FunctionOutcome> run(
    String command, {
    FunctionLimits limits = roomy,
    FunctionCancellation? cancel,
  }) => engine.invoke(
    module: probe,
    index: 0,
    input: utf8Bytes(command),
    limits: limits,
    host: host,
    cancel: cancel,
  );

  FunctionLimits with_({
    Duration? timeout,
    int? memoryBytes,
    int? inputBytes,
    int? outputBytes,
    int? logBytes,
  }) => FunctionLimits(
    timeout: timeout ?? roomy.timeout,
    memoryBytes: memoryBytes ?? roomy.memoryBytes,
    inputBytes: inputBytes ?? roomy.inputBytes,
    outputBytes: outputBytes ?? roomy.outputBytes,
    logBytes: logBytes ?? roomy.logBytes,
  );

  test(
    '[FN-053] an infinite loop ends at the deadline with PLX-7001',
    () async {
      final watch = Stopwatch()..start();
      await expectLater(
        run('loop', limits: with_(timeout: const Duration(milliseconds: 200))),
        throwsA(plux(PluxErrorCode.functionTimeout)),
      );
      expect(watch.elapsed, lessThan(const Duration(seconds: 3)));
    },
  );

  test(
    '[FN-053] the engine runs the next invocation after a timeout',
    () async {
      await expectLater(
        run('loop', limits: with_(timeout: const Duration(milliseconds: 100))),
        throwsA(plux(PluxErrorCode.functionTimeout)),
      );
      expect(outputText(await run('echo again')), '"again"');
    },
  );

  test(
    '[FN-053] cancelling a running invocation ends it with PLX-7021',
    () async {
      final token = FunctionCancellation();
      final running = run(
        'loop',
        limits: with_(timeout: const Duration(seconds: 30)),
        cancel: token,
      );
      final failure = expectLater(
        running,
        throwsA(plux(PluxErrorCode.functionCancelled)),
      );
      await Future<void>.delayed(const Duration(milliseconds: 200));
      final watch = Stopwatch()..start();
      token.cancel();
      await failure;
      expect(watch.elapsed, lessThan(const Duration(seconds: 3)));
    },
  );

  test('[FN-053] a token cancelled before the call stops it at once', () async {
    final token = FunctionCancellation()..cancel();
    await expectLater(
      run('echo never', cancel: token),
      throwsA(plux(PluxErrorCode.functionCancelled)),
    );
  });

  test(
    '[FN-053] cancelling a queued invocation stops it before it runs',
    () async {
      final token = FunctionCancellation();
      final first = run(
        'loop',
        limits: with_(timeout: const Duration(milliseconds: 400)),
      );
      final queued = run('echo queued', cancel: token);
      token.cancel();
      await expectLater(first, throwsA(plux(PluxErrorCode.functionTimeout)));
      await expectLater(queued, throwsA(plux(PluxErrorCode.functionCancelled)));
    },
  );

  test(
    '[FN-053] a guest that allocates past the cap fails with PLX-7002',
    () async {
      await expectLater(
        run('alloc 67108864', limits: with_(memoryBytes: 8 * 1024 * 1024)),
        throwsA(plux(PluxErrorCode.functionMemoryExceeded)),
      );
    },
  );

  test('[FN-053] the same allocation passes under a larger cap', () async {
    expect(
      outputText(
        await run(
          'alloc 4194304',
          limits: with_(memoryBytes: 16 * 1024 * 1024),
        ),
      ),
      '4194304',
    );
  });

  test(
    '[FN-053] a cap below the module\'s initial memory fails with PLX-7002',
    () async {
      await expectLater(
        run('echo x', limits: with_(memoryBytes: 256 * 1024)),
        throwsA(plux(PluxErrorCode.functionMemoryExceeded)),
      );
    },
  );

  test('[FN-053] a trap fails with PLX-7007 and the engine survives', () async {
    await expectLater(
      run('trap'),
      throwsA(plux(PluxErrorCode.functionTrapped)),
    );
    expect(outputText(await run('echo alive')), '"alive"');
  });

  test('[FN-053] runaway recursion traps with PLX-7007', () async {
    await expectLater(
      run('deep 1000000'),
      throwsA(plux(PluxErrorCode.functionTrapped)),
    );
  });

  test(
    '[FN-052] proc_exit ends the call as a trap that names the code',
    () async {
      await expectLater(
        run('exit 3'),
        throwsA(
          isA<PluxException>()
              .having((e) => e.code, 'code', PluxErrorCode.functionTrapped)
              .having((e) => e.message, 'message', contains('proc_exit(3)')),
        ),
      );
    },
  );

  test('[FN-053] an output over the limit fails with PLX-7004', () async {
    await expectLater(
      run('big 1000', limits: with_(outputBytes: 100)),
      throwsA(plux(PluxErrorCode.functionOutputTooLarge)),
    );
    expect(
      outputText(await run('big 100', limits: with_(outputBytes: 100))).length,
      100,
    );
  });

  test(
    '[FN-053] an input over the limit fails with PLX-7022 before it runs',
    () async {
      await expectLater(
        run('echo 0123456789', limits: with_(inputBytes: 5)),
        throwsA(plux(PluxErrorCode.functionInputTooLarge)),
      );
    },
  );

  test('[FN-022] a business error comes back as an outcome', () async {
    final outcome = await run('business');
    expect(outcome, isA<FunctionBusinessError>());
    final error = outcome as FunctionBusinessError;
    expect(error.code, 'PROBE_ERROR');
    expect(error.messageKey, 'probe.error');
    expect(error.params, {'n': 3});
  });

  test(
    '[FN-053] the guest refusing its input is PLX-7008 with its message',
    () async {
      await expectLater(
        run('refuse'),
        throwsA(
          isA<PluxException>()
              .having((e) => e.code, 'code', PluxErrorCode.functionInputInvalid)
              .having((e) => e.message, 'message', 'probe refused its input'),
        ),
      );
    },
  );

  test('[FN-053] the guest failing to encode its output is PLX-7009', () async {
    await expectLater(
      run('unencodable'),
      throwsA(plux(PluxErrorCode.functionOutputInvalid)),
    );
  });

  test('[FN-010] every invocation starts on a fresh instance', () async {
    expect(outputText(await run('counter')), '1');
    expect(outputText(await run('counter')), '1');
  });

  test('[FN-053] invocations run in order on the one worker', () async {
    final results = await Future.wait([
      for (var i = 0; i < 5; i++) run('echo $i'),
    ]);
    expect(results.map(outputText), ['"0"', '"1"', '"2"', '"3"', '"4"']);
  });

  test(
    '[FN-053] a closed engine fails invocations instead of hanging',
    () async {
      final other = await FunctionEngine.start();
      final module = await other.load(key: 'p', bytes: fixture('probe.wasm'));
      await other.close();
      await expectLater(
        other.invoke(
          module: module,
          index: 0,
          input: utf8Bytes('echo x'),
          limits: roomy,
          host: host,
        ),
        throwsA(isA<PluxException>()),
      );
    },
  );
}
