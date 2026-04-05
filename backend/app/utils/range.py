from starlette.datastructures import Headers
from starlette.requests import Request
from typing import Optional, Tuple


def parse_range_header(header: Optional[str], file_size: int) -> Optional[Tuple[int, int]]:
    """Parse HTTP Range header. Returns (start, end) inclusive byte positions."""
    if not header:
        return None
    if not header.startswith("bytes="):
        return None
    ranges = header[6:].split(",")
    if not ranges:
        return None
    parts = ranges[0].split("-")
    if len(parts) != 2:
        return None
    start_str, end_str = parts
    if start_str == "":
        # suffix range: last N bytes
        if end_str == "":
            return None
        end = int(end_str)
        start = max(0, file_size - end)
        return (start, file_size - 1)
    start = int(start_str)
    if end_str == "":
        return (start, file_size - 1)
    end = int(end_str)
    if start > end or start >= file_size:
        return None
    return (start, min(end, file_size - 1))


def get_range_info(request: Request, file_size: int) -> Tuple[int, int, int]:
    """Returns (start, end, total). If no Range header, returns (0, file_size-1, file_size)."""
    range_header = request.headers.get("range")
    parsed = parse_range_header(range_header, file_size)
    if parsed:
        return (*parsed, file_size)
    return (0, file_size - 1, file_size)
