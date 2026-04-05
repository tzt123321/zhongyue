"""add effect_presets table

Revision ID: e95f7d27b7ec
Revises: 002_track_artists
Create Date: 2026-03-27 11:37:00.756742

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'e95f7d27b7ec'
down_revision: Union[str, None] = '002_track_artists'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table('effect_presets',
    sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
    sa.Column('user_id', sa.Integer(), nullable=False),
    sa.Column('name', sa.String(length=100), nullable=False),
    sa.Column('is_system', sa.Boolean(), nullable=False),
    sa.Column('eq_low', sa.Float(), nullable=True),
    sa.Column('eq_mid', sa.Float(), nullable=True),
    sa.Column('eq_high', sa.Float(), nullable=True),
    sa.Column('balance', sa.Float(), nullable=True),
    sa.ForeignKeyConstraint(['user_id'], ['users.id'], ondelete='CASCADE'),
    sa.PrimaryKeyConstraint('id')
    )


def downgrade() -> None:
    op.drop_table('effect_presets')
