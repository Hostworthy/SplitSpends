import 'dart:async';
import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart' show Override;
import 'package:flutter_test/flutter_test.dart';
import 'package:image_picker/image_picker.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/app.dart';
import 'package:splitspends/features/receipts/data/providers.dart';
import 'package:splitspends/app/router.dart';
import 'package:splitspends/core/routes.dart';
import 'package:splitspends/core/api/api_error.dart';
import 'package:splitspends/core/auth/local_store.dart';
import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/features/expenses/domain/expense.dart';
import 'package:splitspends/features/receipts/domain/receipt.dart';
import 'package:splitspends/features/receipts/domain/receipts_repository.dart';
import 'package:splitspends/features/receipts/presentation/ai_quick_add_sheet.dart';
import 'package:splitspends/features/receipts/presentation/upload_receipt.dart';

import '../helpers.dart';

/// A 1×1 transparent PNG.
final _png = Uint8List.fromList(const [
  0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, //
  0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, //
  0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00, //
  0x0D, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00, //
  0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49, //
  0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82, //
]);

/// Always "picks" the same small photo.
class _Picker extends ImagePicker {
  @override
  Future<XFile?> pickImage({
    required ImageSource source,
    double? maxWidth,
    double? maxHeight,
    int? imageQuality,
    CameraDevice preferredCameraDevice = CameraDevice.rear,
    bool requestFullMetadata = true,
  }) async => XFile.fromData(_png, name: 'receipt.jpg', mimeType: 'image/jpeg');
}

/// Uploads succeed; drafts come from [draftResult].
class _Receipts implements ReceiptsRepository {
  _Receipts(this.draftResult);

  final Future<ExpenseInput> Function() draftResult;

  @override
  Future<Receipt> upload(
    Uint8List bytes,
    String filename,
    String contentType,
  ) async => const Receipt(id: 'r1');

  @override
  Future<Uint8List> image(String id) async => Uint8List(0);

  @override
  Future<ExpenseInput> draft(
    String text, {
    String? groupId,
    String? receiptId,
  }) => draftResult();
}

/// Starts the app on the device as Ada.
Future<ProviderContainer> _start(
  WidgetTester tester,
  List<Override> overrides,
) async {
  tester.view.physicalSize = const Size(800, 2400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    ProviderScope(
      retry: (_, _) => null,
      overrides: [
        localStoreProvider.overrideWithValue(MemoryLocalStore()),
        memoryDatabase(),
        ...overrides,
      ],
      child: const SplitSpendsApp(),
    ),
  );
  await settle(tester);
  await tester.tap(find.text('Start'));
  await settle(tester);
  await tester.enterText(find.byKey(const Key('local-name')), 'Ada');
  await tester.tap(find.byKey(const Key('local-start')));
  await settle(tester);
  return ProviderScope.containerOf(tester.element(find.byType(SplitSpendsApp)));
}

void main() {
  testWidgets('scan shows an error when the draft cannot be parsed', (
    tester,
  ) async {
    final container = await _start(tester, [
      imagePickerProvider.overrideWithValue(_Picker()),
      receiptsRepositoryProvider.overrideWithValue(
        _Receipts(() async => throw TypeError()),
      ),
    ]);
    unawaited(container.read(routerProvider).push(Routes.scan));
    await settle(tester);

    await tester.tap(find.text('Choose from library'));
    await settle(tester);

    expect(find.text('Reading the receipt…'), findsNothing);
    expect(find.text('Something went wrong.'), findsOneWidget);
    final library = tester.widget<OutlinedButton>(
      find.ancestor(
        of: find.text('Choose from library'),
        matching: find.byWidgetPredicate((w) => w is OutlinedButton),
      ),
    );
    expect(library.onPressed, isNotNull);
  });

  testWidgets('closing the quick-add sheet during a request does not throw', (
    tester,
  ) async {
    final reply = Completer<ExpenseInput>();
    await _start(tester, [
      receiptsRepositoryProvider.overrideWithValue(
        _Receipts(() => reply.future),
      ),
    ]);
    unawaited(showAiQuickAddSheet(tester.element(find.byType(Scaffold).first)));
    await settle(tester);
    await tester.enterText(find.byType(TextField), 'Dinner 60');
    await tester.tap(find.text('Create draft'));
    await tester.pump();

    await tester.tapAt(const Offset(200, 20));
    await settle(tester);
    expect(find.text('Create draft'), findsNothing);

    reply.completeError(const ApiError.network('Cannot reach the server.'));
    await settle(tester);
    expect(tester.takeException(), isNull);
  });
}
