"""add track_artists table

Revision ID: 002_track_artists
Revises: 001_initial
Create Date: 2026-03-27

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


revision: str = '002_track_artists'
down_revision: Union[str, None] = '001_initial'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table(
        'track_artists',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('track_id', sa.Integer(), nullable=False),
        sa.Column('artist_id', sa.Integer(), nullable=False),
        sa.Column('is_primary', sa.Boolean(), server_default='0', nullable=False),
        sa.ForeignKeyConstraint(['track_id'], ['tracks.id'], ondelete='CASCADE'),
        sa.ForeignKeyConstraint(['artist_id'], ['artists.id'], ondelete='CASCADE'),
        sa.PrimaryKeyConstraint('id'),
    )
    op.create_index('idx_track_artists_track_id', 'track_artists', ['track_id'])
    op.create_index('idx_track_artists_artist_id', 'track_artists', ['artist_id'])


def downgrade() -> None:
    op.drop_index('idx_track_artists_artist_id', table_name='track_artists')
    op.drop_index('idx_track_artists_track_id', table_name='track_artists')
    op.drop_table('track_artists')
