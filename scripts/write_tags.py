#!/usr/bin/env python3
"""
write_tags.py - Write audio file metadata (title/artist/album/cover art, etc.)
Uses mid3v2 for MP3/WAV, mutagen for FLAC/OGG/MP4.

Usage:
  python3 write_tags.py <file> [--title X] [--artist X] [--album X]
                               [--album-artist X] [--year N] [--genre X]
                               [--track N] [--disc N]
                               [--lyrics TEXT] [--cover-url URL]
"""
import sys
import os
import subprocess
import tempfile
import urllib.request
import base64
import struct
import argparse


def download_image(url):
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "ZhongYue/1.0"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.read()
    except Exception as e:
        print(f"WARN: image download failed: {e}", file=sys.stderr)
        return None


def save_temp_image(cover_bytes):
    """Write bytes to a temp file, return path."""
    fd, path = tempfile.mkstemp(suffix=".jpg")
    os.write(fd, cover_bytes)
    os.close(fd)
    return path


# ─── mid3v2 helpers (MP3 / WAV) ────────────────────────────────────────────

def write_mid3v2(file_path, title=None, artist=None, album=None,
                 album_artist=None, year=None, genre=None,
                 track_number=None, disc_number=None, lyrics=None,
                 cover_bytes=None):
    cmd = ["mid3v2"]
    for flag, val in [
        ("-t", title),
        ("-a", artist),
        ("-A", album),
        ("-T", album_artist),
        ("-y", str(year) if year else None),
        ("-g", genre),
        ("-n", str(track_number) if track_number is not None else None),
        ("-d", str(disc_number) if disc_number is not None else None),
    ]:
        if val is not None:
            cmd.extend([flag, val])
    cmd.append(file_path)
    subprocess.run(cmd, check=False)

    if lyrics:
        subprocess.run(["mid3v2", "--comment", f":::{lyrics}", file_path], check=False)

    if cover_bytes:
        tmp_path = save_temp_image(cover_bytes)
        # Format: FILENAME:DESCRIPTION:IMAGE-TYPE:MIME-TYPE
        # IMAGE-TYPE 3 = front cover
        subprocess.run(["mid3v2", f"--picture={tmp_path}:Cover:3:image/jpeg", file_path], check=False)
        os.unlink(tmp_path)

    print(f"OK (mid3v2): {file_path}")


# ─── FLAC via mutagen ───────────────────────────────────────────────────────

def write_flac(file_path, title=None, artist=None, album=None,
               album_artist=None, year=None, genre=None,
               track_number=None, disc_number=None, lyrics=None,
               cover_bytes=None):
    from mutagen.flac import FLAC, Picture
    audio = FLAC(file_path)
    if title:
        audio["TITLE"] = [title]
    if artist:
        audio["ARTIST"] = [artist]
    if album:
        audio["ALBUM"] = [album]
    if album_artist:
        audio["ALBUMARTIST"] = [album_artist]
    if year:
        audio["DATE"] = [str(year)]
    if genre:
        audio["GENRE"] = [genre]
    if track_number is not None:
        audio["TRACKNUMBER"] = [str(track_number)]
    if disc_number is not None:
        audio["DISCNUMBER"] = [str(disc_number)]
    if lyrics:
        audio["LYRICS"] = [lyrics]

    if cover_bytes:
        for p in list(audio.pictures):
            if p.type == 3:
                audio.remove(p)
        pic = Picture()
        pic.type = 3
        pic.mime = "image/jpeg"
        pic.desc = "Cover"
        pic.data = cover_bytes
        audio.add_picture(pic)

    audio.save()
    print(f"OK (mutagen FLAC): {file_path}")


# ─── OGG Vorbis via mutagen ─────────────────────────────────────────────────

def write_ogg(file_path, title=None, artist=None, album=None,
              album_artist=None, year=None, genre=None,
              track_number=None, disc_number=None, cover_bytes=None):
    from mutagen.oggvorbis import OggVorbis
    audio = OggVorbis(file_path)
    if title:
        audio["TITLE"] = [title]
    if artist:
        audio["ARTIST"] = [artist]
    if album:
        audio["ALBUM"] = [album]
    if album_artist:
        audio["ALBUMARTIST"] = [album_artist]
    if year:
        audio["DATE"] = [str(year)]
    if genre:
        audio["GENRE"] = [genre]
    if track_number is not None:
        audio["TRACKNUMBER"] = [str(track_number)]
    if disc_number is not None:
        audio["DISCNUMBER"] = [str(disc_number)]

    if cover_bytes:
        # Build METADATA_BLOCK_PICTURE for OGG/Vorbis Comment
        # struct: type(3) mime_len desc_len url_len data
        pic_type = 3
        mime_bytes = b"image/jpeg"
        desc_bytes = b"Cover"
        blob = struct.pack('>III', pic_type, len(mime_bytes), len(desc_bytes))
        blob += mime_bytes + desc_bytes + b'' + cover_bytes
        audio["METADATA_BLOCK_PICTURE"] = [base64.b64encode(blob).decode('ascii')]

    audio.save()
    print(f"OK (mutagen OGG): {file_path}")


# ─── MP4/M4A/AAC via mutagen ────────────────────────────────────────────────

def write_mp4(file_path, title=None, artist=None, album=None,
              album_artist=None, year=None, genre=None,
              track_number=None, disc_number=None, lyrics=None,
              cover_bytes=None):
    from mutagen.mp4 import MP4
    audio = MP4(file_path)
    if title:
        audio["\xa9nam"] = [title]
    if artist:
        audio["\xa9ART"] = [artist]
    if album:
        audio["\xa9alb"] = [album]
    if album_artist:
        audio["aART"] = [album_artist]
    if year:
        audio["\xa9day"] = [str(year)]
    if genre:
        audio["\xa9gen"] = [genre]
    if track_number is not None:
        audio["trkn"] = [(track_number, 0)]
    if disc_number is not None:
        audio["disk"] = [(disc_number, 0)]
    if lyrics:
        audio["\xa9lyr"] = [lyrics]

    if cover_bytes:
        if "covr" in audio:
            del audio["covr"]
        audio["covr"] = [cover_bytes]

    audio.save()
    print(f"OK (mutagen MP4): {file_path}")


# ─── Dispatcher ────────────────────────────────────────────────────────────

def write_tags(file_path, title=None, artist=None, album=None,
               album_artist=None, year=None, genre=None,
               track_number=None, disc_number=None,
               lyrics=None, cover_url=None):
    ext = os.path.splitext(file_path)[-1].lower()
    cover_bytes = download_image(cover_url) if cover_url else None

    if ext == '.mp3':
        write_mid3v2(file_path, title, artist, album, album_artist,
                     year, genre, track_number, disc_number, lyrics, cover_bytes)
    elif ext == '.flac':
        write_flac(file_path, title, artist, album, album_artist,
                   year, genre, track_number, disc_number, lyrics, cover_bytes)
    elif ext == '.ogg':
        write_ogg(file_path, title, artist, album, album_artist,
                  year, genre, track_number, disc_number, cover_bytes)
    elif ext in ('.m4a', '.mp4', '.aac'):
        write_mp4(file_path, title, artist, album, album_artist,
                  year, genre, track_number, disc_number, lyrics, cover_bytes)
    elif ext == '.wav':
        write_mid3v2(file_path, title, artist, album, album_artist,
                     year, genre, track_number, disc_number, None, cover_bytes)
    else:
        print(f"WARN: unsupported format {ext}")
        return False
    return True


def main():
    parser = argparse.ArgumentParser(description='Write audio metadata tags')
    parser.add_argument('file_path', help='Path to audio file')
    parser.add_argument('--title')
    parser.add_argument('--artist')
    parser.add_argument('--album')
    parser.add_argument('--album-artist')
    parser.add_argument('--year', type=int)
    parser.add_argument('--genre')
    parser.add_argument('--track', type=int)
    parser.add_argument('--disc', type=int)
    parser.add_argument('--lyrics')
    parser.add_argument('--cover-url')
    args = parser.parse_args()

    success = write_tags(
        args.file_path,
        title=args.title, artist=args.artist, album=args.album,
        album_artist=args.album_artist, year=args.year, genre=args.genre,
        track_number=args.track, disc_number=args.disc,
        lyrics=args.lyrics, cover_url=args.cover_url,
    )
    sys.exit(0 if success else 1)


if __name__ == '__main__':
    main()