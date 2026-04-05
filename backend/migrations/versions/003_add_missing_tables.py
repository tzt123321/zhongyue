"""add invite_codes, notifications, and missing user columns

Revision ID: 003_add_missing_tables
Revises: 606e15fed784
Create Date: 2026-03-27 13:25:00

"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa

revision: str = '003_add_missing_tables'
down_revision: Union[str, None] = '606e15fed784'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None

def upgrade() -> None:
    # users: add missing columns
    op.add_column('users', sa.Column('is_active', sa.Boolean(), nullable=False, server_default=sa.text('1')))
    op.add_column('users', sa.Column('is_banned', sa.Boolean(), nullable=False, server_default=sa.text('0')))
    op.add_column('users', sa.Column('is_musician', sa.Boolean(), nullable=False, server_default=sa.text('0')))
    op.add_column('users', sa.Column('invite_code', sa.String(length=32), nullable=True))
    op.add_column('users', sa.Column('initial_password_hash', sa.String(length=255), nullable=True))
    op.add_column('users', sa.Column('initial_password', sa.String(length=128), nullable=True))

    # invite_codes table
    op.create_table(
        'invite_codes',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('code', sa.String(length=64), nullable=False),
        sa.Column('created_by', sa.Integer(), nullable=False),
        sa.Column('used_by', sa.Integer(), nullable=True),
        sa.Column('used', sa.Boolean(), nullable=False, server_default=sa.text('0')),
        sa.Column('max_uses', sa.Integer(), nullable=False, server_default=sa.text('1')),
        sa.Column('uses', sa.Integer(), nullable=False, server_default=sa.text('0')),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.ForeignKeyConstraint('created_by', ['users.id']),
        sa.ForeignKeyConstraint('used_by', ['users.id']),
    )
    op.create_index('ix_invite_codes_code', 'invite_codes', ['code'], unique=True)

    # notifications table
    op.create_table(
        'notifications',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('user_id', sa.Integer(), nullable=False),
        sa.Column('type', sa.String(length=50), nullable=False),
        sa.Column('title', sa.String(length=255), nullable=False),
        sa.Column('content', sa.Text(), nullable=False),
        sa.Column('is_read', sa.Boolean(), nullable=False, server_default=sa.text('0')),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=True),
        sa.ForeignKeyConstraint('user_id', ['users.id'], ondelete='CASCADE'),
    )

def downgrade() -> None:
    op.drop_table('notifications')
    op.drop_table('invite_codes')
    op.drop_column('users', 'initial_password')
    op.drop_column('users', 'initial_password_hash')
    op.drop_column('users', 'invite_code')
    op.drop_column('users', 'is_musician')
    op.drop_column('users', 'is_banned')
    op.drop_column('users', 'is_active')
