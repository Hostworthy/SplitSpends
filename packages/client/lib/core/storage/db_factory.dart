import 'package:sembast/sembast.dart';

import 'package:splitspends/core/storage/db_factory_io.dart'
    if (dart.library.js_interop) 'package:splitspends/core/storage/db_factory_web.dart'
    as impl;

/// Opens the app database: IndexedDB on the web, a file in the app's
/// documents directory on phones.
Future<Database> openAppDatabase(String name) => impl.openAppDatabase(name);
