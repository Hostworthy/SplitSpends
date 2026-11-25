import 'dart:io';

import 'package:http/http.dart' as http;

bool isTransportFailure(Object error) =>
    error is http.ClientException || error is IOException;

bool isTlsFailure(Object error) => error is TlsException;
