/// Exact fractions on [BigInt], the Dart counterpart of Go's `big.Rat`.
/// Money math never touches floating point.
class Rational implements Comparable<Rational> {
  factory Rational(BigInt numerator, [BigInt? denominator]) {
    var n = numerator;
    var d = denominator ?? BigInt.one;
    if (d == BigInt.zero) throw ArgumentError('zero denominator');
    if (d.isNegative) {
      n = -n;
      d = -d;
    }
    final g = n.gcd(d);
    if (g > BigInt.one) {
      n = n ~/ g;
      d = d ~/ g;
    }
    return Rational._(n, d);
  }

  const Rational._(this.numerator, this.denominator);

  Rational.integer(int value) : this._(BigInt.from(value), BigInt.one);

  static final zero = Rational.integer(0);

  static final _plain = RegExp(r'^[+-]?(\d+(\.\d*)?|\.\d+)$');

  /// Parses a plain decimal like `12`, `-3.5` or `.25`. Returns null for
  /// anything else (fractions, exponents, blanks).
  static Rational? tryParseDecimal(String input) {
    final s = input.trim();
    if (!_plain.hasMatch(s)) return null;
    var negative = false;
    var body = s;
    if (body.startsWith('+') || body.startsWith('-')) {
      negative = body.startsWith('-');
      body = body.substring(1);
    }
    final dot = body.indexOf('.');
    final whole = dot < 0 ? body : body.substring(0, dot);
    final frac = dot < 0 ? '' : body.substring(dot + 1);
    final digits = '${whole.isEmpty ? '0' : whole}$frac';
    final n = BigInt.parse(digits);
    return Rational(negative ? -n : n, BigInt.from(10).pow(frac.length));
  }

  final BigInt numerator;
  final BigInt denominator;

  bool get isInteger => denominator == BigInt.one;
  int get sign => numerator.sign;

  Rational operator +(Rational o) => Rational(
    numerator * o.denominator + o.numerator * denominator,
    denominator * o.denominator,
  );

  Rational operator -(Rational o) => Rational(
    numerator * o.denominator - o.numerator * denominator,
    denominator * o.denominator,
  );

  Rational operator *(Rational o) =>
      Rational(numerator * o.numerator, denominator * o.denominator);

  Rational operator /(Rational o) =>
      Rational(numerator * o.denominator, denominator * o.numerator);

  /// Floor toward negative infinity for negatives is not needed here; this is
  /// truncation, which equals floor for the non-negative values we use.
  BigInt truncate() => numerator ~/ denominator;

  @override
  int compareTo(Rational o) =>
      (numerator * o.denominator).compareTo(o.numerator * denominator);

  @override
  bool operator ==(Object other) =>
      other is Rational &&
      other.numerator == numerator &&
      other.denominator == denominator;

  @override
  int get hashCode => Object.hash(numerator, denominator);

  @override
  String toString() => isInteger ? '$numerator' : '$numerator/$denominator';
}
