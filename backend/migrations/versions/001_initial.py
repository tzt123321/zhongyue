"""Initial migration - create all tables

Revision ID: 001_initial
Revises:
Create Date: 2026-03-23

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


revision: str = '001_initial'
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Users table
    op.create_table(
        'users',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('username', sa.String(length=50), nullable=False),
        sa.Column('email', sa.String(length=255), nullable=True),
        sa.Column('password_hash', sa.String(length=255), nullable=False),
        sa.Column('is_admin', sa.Boolean(), server_default='0', nullable=False),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.Column('updated_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('username'),
        sa.UniqueConstraint('email'),
    )

    # Artists table
    op.create_table(
        'artists',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('name', sa.String(length=255), nullable=False),
        sa.Column('sort_name', sa.String(length=255), nullable=True),
        sa.Column('musicbrainz_id', sa.String(length=36), nullable=True),
        sa.Column('biography', sa.Text(), nullable=True),
        sa.Column('cover_path', sa.String(length=500), nullable=True),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.PrimaryKeyConstraint('id'),
    )
    op.create_index('idx_artists_name', 'artists', ['name'])

    # Albums table
    op.create_table(
        'albums',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('name', sa.String(length=255), nullable=False),
        sa.Column('artist_id', sa.Integer(), nullable=True),
        sa.Column('year', sa.Integer(), nullable=True),
        sa.Column('cover_path', sa.String(length=500), nullable=True),
        sa.Column('total_tracks', sa.Integer(), server_default='0', nullable=False),
        sa.Column('musicbrainz_id', sa.String(length=36), nullable=True),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.ForeignKeyConstraint(['artist_id'], ['artists.id'], ondelete='SET NULL'),
        sa.PrimaryKeyConstraint('id'),
    )
    op.create_index('idx_albums_name', 'albums', ['name'])
    op.create_index('idx_albums_artist_id', 'albums', ['artist_id'])

    # Tracks table
    op.create_table(
        'tracks',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('title', sa.String(length=500), nullable=False),
        sa.Column('artist_id', sa.Integer(), nullable=True),
        sa.Column('album_id', sa.Integer(), nullable=True),
        sa.Column('track_number', sa.Integer(), nullable=True),
        sa.Column('disc_number', sa.Integer(), nullable=True),
        sa.Column('duration', sa.Integer(), nullable=True),
        sa.Column('bitrate', sa.Integer(), nullable=True),
        sa.Column('format', sa.String(length=10), nullable=True),
        sa.Column('file_path', sa.String(length=1000), nullable=False),
        sa.Column('file_size', sa.BigInteger(), nullable=True),
        sa.Column('file_mtime', sa.DateTime(), nullable=True),
        sa.Column('play_count', sa.Integer(), server_default='0', nullable=False),
        sa.Column('last_played', sa.DateTime(), nullable=True),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.Column('updated_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.ForeignKeyConstraint(['artist_id'], ['artists.id'], ondelete='SET NULL'),
        sa.ForeignKeyConstraint(['album_id'], ['albums.id'], ondelete='SET NULL'),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('file_path'),
    )
    op.create_index('idx_tracks_title', 'tracks', ['title'])
    op.create_index('idx_tracks_artist_id', 'tracks', ['artist_id'])
    op.create_index('idx_tracks_album_id', 'tracks', ['album_id'])

    # FTS5 virtual table for full-text search
    op.execute('''
        CREATE VIRTUAL TABLE IF NOT EXISTS tracks_fts USING fts5(
            title, artist_name, album_name
        )
    ''')

    # Triggers to keep FTS in sync
    op.execute('''
        CREATE TRIGGER IF NOT EXISTS tracks_ai AFTER INSERT ON tracks BEGIN
            INSERT INTO tracks_fts(rowid, title, artist_name, album_name)
            SELECT new.id, new.title,
                COALESCE((SELECT name FROM artists WHERE id = new.artist_id), ''),
                COALESCE((SELECT name FROM albums WHERE id = new.album_id), '');
        END
    ''')
    op.execute('''
        CREATE TRIGGER IF NOT EXISTS tracks_ad AFTER DELETE ON tracks BEGIN
            DELETE FROM tracks_fts WHERE rowid = old.id;
        END
    ''')
    op.execute('''
        CREATE TRIGGER IF NOT EXISTS tracks_au AFTER UPDATE ON tracks BEGIN
            DELETE FROM tracks_fts WHERE rowid = old.id;
            INSERT INTO tracks_fts(rowid, title, artist_name, album_name)
            SELECT new.id, new.title,
                COALESCE((SELECT name FROM artists WHERE id = new.artist_id), ''),
                COALESCE((SELECT name FROM albums WHERE id = new.album_id), '');
        END
    ''')

    # Playlists table
    op.create_table(
        'playlists',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('name', sa.String(length=255), nullable=False),
        sa.Column('description', sa.Text(), nullable=True),
        sa.Column('user_id', sa.Integer(), nullable=True),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.Column('updated_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.ForeignKeyConstraint(['user_id'], ['users.id'], ondelete='CASCADE'),
        sa.PrimaryKeyConstraint('id'),
    )

    # Playlist tracks junction table
    op.create_table(
        'playlist_tracks',
        sa.Column('playlist_id', sa.Integer(), nullable=False),
        sa.Column('track_id', sa.Integer(), nullable=False),
        sa.Column('position', sa.Integer(), nullable=False),
        sa.Column('added_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.ForeignKeyConstraint(['playlist_id'], ['playlists.id'], ondelete='CASCADE'),
        sa.ForeignKeyConstraint(['track_id'], ['tracks.id'], ondelete='CASCADE'),
        sa.PrimaryKeyConstraint('playlist_id', 'track_id'),
    )

    # Play history table
    op.create_table(
        'play_history',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('user_id', sa.Integer(), nullable=True),
        sa.Column('track_id', sa.Integer(), nullable=True),
        sa.Column('played_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.ForeignKeyConstraint(['user_id'], ['users.id'], ondelete='CASCADE'),
        sa.ForeignKeyConstraint(['track_id'], ['tracks.id'], ondelete='CASCADE'),
        sa.PrimaryKeyConstraint('id'),
    )
    op.create_index('idx_play_history_user_id', 'play_history', ['user_id'])

    # Settings table
    op.create_table(
        'settings',
        sa.Column('key', sa.String(length=100), nullable=False),
        sa.Column('value', sa.Text(), nullable=True),
        sa.Column('updated_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.PrimaryKeyConstraint('key'),
    )

    # Default settings
    op.execute("INSERT INTO settings (key, value) VALUES ('library_path', '')")
    op.execute("INSERT INTO settings (key, value) VALUES ('scan_interval', '3600')")
    op.execute("INSERT INTO settings (key, value) VALUES ('theme', 'dark')")


def downgrade() -> None:
    op.execute('DROP TRIGGER IF EXISTS tracks_au')
    op.execute('DROP TRIGGER IF EXISTS tracks_ad')
    op.execute('DROP TRIGGER IF EXISTS tracks_ai')
    op.execute('DROP TABLE IF EXISTS tracks_fts')
    op.drop_table('settings')
    op.drop_index('idx_play_history_user_id', table_name='play_history')
    op.drop_table('play_history')
    op.drop_table('playlist_tracks')
    op.drop_table('playlists')
    op.drop_index('idx_tracks_album_id', table_name='tracks')
    op.drop_index('idx_tracks_artist_id', table_name='tracks')
    op.drop_index('idx_tracks_title', table_name='tracks')
    op.drop_table('tracks')
    op.drop_index('idx_albums_artist_id', table_name='albums')
    op.drop_index('idx_albums_name', table_name='albums')
    op.drop_table('albums')
    op.drop_index('idx_artists_name', table_name='artists')
    op.drop_table('artists')
    op.drop_table('users')
