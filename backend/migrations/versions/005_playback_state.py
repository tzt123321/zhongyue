"""add playback_state table

Revision ID: 005_playback_state
Revises: 004_add_api_key
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa

revision: str = '005_playback_state'
down_revision: Union[str, None] = '004_add_api_key'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table(
        'playback_state',
        sa.Column('id', sa.Integer(), primary_key=True, default=1),
        sa.Column('track_id', sa.Integer(), nullable=True),
        sa.Column('position', sa.Integer(), default=0),
        sa.Column('is_playing', sa.Integer(), default=0),
        sa.Column('volume', sa.Integer(), default=80),
        sa.Column('updated_at', sa.DateTime(), nullable=True),
    )


def downgrade() -> None:
    op.drop_table('playback_state')
