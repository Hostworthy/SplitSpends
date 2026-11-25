import 'package:http/http.dart' as http;

bool isTransportFailure(Object error) => error is http.ClientException;

bool isTlsFailure(Object error) => false;
