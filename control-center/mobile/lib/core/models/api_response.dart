// API Response wrapper types matching the backend contract
// See: control-center/workers/src/types.ts

/// Base API response type
class ApiResponse<T> {
  final bool success;

  ApiResponse({required this.success});
}

/// Successful API response with data
class ApiSuccessResponse<T> extends ApiResponse<T> {
  final T data;

  ApiSuccessResponse({required this.data}) : super(success: true);

  factory ApiSuccessResponse.fromJson(
    Map<String, dynamic> json,
    T Function(dynamic) fromJsonT,
  ) {
    return ApiSuccessResponse<T>(
      data: fromJsonT(json['data']),
    );
  }
}

/// Error API response
class ApiErrorResponse extends ApiResponse<Never> {
  final String error;

  ApiErrorResponse({required this.error}) : super(success: false);

  factory ApiErrorResponse.fromJson(Map<String, dynamic> json) {
    return ApiErrorResponse(
      error: json['error'] as String,
    );
  }
}

/// Message API response (for success messages without data)
class ApiMessageResponse extends ApiResponse<Never> {
  final String message;

  ApiMessageResponse({required this.message}) : super(success: true);

  factory ApiMessageResponse.fromJson(Map<String, dynamic> json) {
    return ApiMessageResponse(
      message: json['message'] as String,
    );
  }
}

/// Validation error response with details
class SchemaValidationErrorResponse extends ApiErrorResponse {
  final List<Map<String, dynamic>> details;

  SchemaValidationErrorResponse({
    required super.error,
    required this.details,
  });

  factory SchemaValidationErrorResponse.fromJson(Map<String, dynamic> json) {
    return SchemaValidationErrorResponse(
      error: json['error'] as String,
      details: (json['details'] as List)
          .map((e) => e as Map<String, dynamic>)
          .toList(),
    );
  }
}