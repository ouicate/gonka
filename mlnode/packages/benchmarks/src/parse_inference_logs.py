#!/usr/bin/env python3
import argparse
import csv
import json
import pathlib
import re
from typing import Dict, List, Tuple


ASSIGNMENT_PATTERN = re.compile(
	# Example: "New Inference started assignedTo=<addr> ... inferenceId=<id>"
	r"New Inference started assignedTo=(?P<generator>\S+) .*? inferenceId=(?P<inference_id>\S+)"
)

VALIDATION_PATTERN = re.compile(
	# Example: "Validating inner loop inferenceId=<id> ... passed=true revalidation=false ... validator=<addr>"
	r"Validating inner loop inferenceId=(?P<inference_id>\S+) .*? passed=(?P<passed>true|false) revalidation=(?P<revalidation>true|false) .*? validator=(?P<validator>\S+)"
)

VALIDATED_STATUS_PATTERN = re.compile(
	# Example: "Saving inference inferenceId=<id> ... status=VALIDATED"
	r"Saving inference inferenceId=(?P<inference_id>\S+) .* status=VALIDATED"
)

SIGNATURE_TYPE0_PATTERN = re.compile(
	# Example: "Validating signature type=0 ... signature=\"<id>\""
	r"Validating signature type=0 .* signature=\"(?P<inference_id>[^\"]+)\""
)

COMPONENTS_PAYLOAD_PATTERN = re.compile(
	# Example: "Components payload=\"{...json...}\" timestamp=..."
	r"Components payload=\"(?P<payload>\{.*?\})\"",
)


def parse_log(
	log_path: pathlib.Path,
) -> List[Dict[str, object]]:
	"""
	Parse the provided log file and return rows for CSV output.

	Columns produced:
	- inference_request_id
	- generator_id
	- first_validator
	- validation_passed (true/false)
	- num_validators_voted (revalidation validators count)
	- num_invalidate_yes (revalidation votes with passed=false)
	- invalidate_yes_addresses (semicolon-separated list)
	"""

	# Aggregations
	inference_id_to_generator: Dict[str, str] = {}
	first_validator_by_inference: Dict[str, str] = {}
	first_passed_by_inference: Dict[str, bool] = {}
	# For revalidation, we treat passed=false as a vote to invalidate
	revalidation_votes: Dict[str, List[Tuple[str, bool]]] = {}
	# Capture prompt per inference (from Components payload linked to type=0 signature)
	prompt_by_inference: Dict[str, str] = {}
	# Track pending type=0 signature so the next Components payload can be attributed
	pending_signature_inference_id: str = ""

	with log_path.open("r", encoding="utf-8", errors="ignore") as fh:
		for line in fh:
			# Keep a lightweight state machine to associate payloads with the prior type=0 signature
			sig_match = SIGNATURE_TYPE0_PATTERN.search(line)
			if sig_match:
				pending_signature_inference_id = sig_match.group("inference_id")
				# continue scanning; payload likely on the next line
				# do not 'continue' to allow same line to be checked for other patterns as well

			if pending_signature_inference_id and pending_signature_inference_id not in prompt_by_inference:
				payload_match = COMPONENTS_PAYLOAD_PATTERN.search(line)
				if payload_match:
					payload_str = payload_match.group("payload")
					try:
						obj = json.loads(payload_str)
						prompt = ""
						msgs = obj.get("messages")
						if isinstance(msgs, list) and msgs:
							# Prefer first user message; fallback to first message content
							user_msg = next((m for m in msgs if isinstance(m, dict) and m.get("role") == "user"), msgs[0])
							if isinstance(user_msg, dict):
								prompt = str(user_msg.get("content", ""))
						prompt_by_inference[pending_signature_inference_id] = prompt
						# Reset pending id after consuming a payload
						pending_signature_inference_id = ""
					except Exception:
						# If payload JSON can't be parsed, leave prompt empty and reset state
						prompt_by_inference[pending_signature_inference_id] = ""
						pending_signature_inference_id = ""

			assignment_match = ASSIGNMENT_PATTERN.search(line)
			if assignment_match:
				inference_id = assignment_match.group("inference_id")
				generator_id = assignment_match.group("generator")
				inference_id_to_generator[inference_id] = generator_id
				continue

			validation_match = VALIDATION_PATTERN.search(line)
			if validation_match:
				inference_id = validation_match.group("inference_id")
				passed = validation_match.group("passed") == "true"
				revalidation = validation_match.group("revalidation") == "true"
				validator = validation_match.group("validator")

				if not revalidation:
					# First time we encounter revalidation=false per inference is the first validation
					if inference_id not in first_validator_by_inference:
						first_validator_by_inference[inference_id] = validator
						first_passed_by_inference[inference_id] = passed
				else:
					votes = revalidation_votes.setdefault(inference_id, [])
					votes.append((validator, passed))
				continue

			validated_status_match = VALIDATED_STATUS_PATTERN.search(line)
			if validated_status_match:
				inference_id = validated_status_match.group("inference_id")
				# If we see a final VALIDATED status but missed the first validation line,
				# mark as passed with unknown validator.
				first_passed_by_inference.setdefault(inference_id, True)

	# Build final rows
	all_inference_ids = (
		set(inference_id_to_generator.keys())
		| set(first_validator_by_inference.keys())
		| set(first_passed_by_inference.keys())
		| set(revalidation_votes.keys())
		| set(prompt_by_inference.keys())
	)

	rows: List[Dict[str, object]] = []
	for inference_id in sorted(all_inference_ids):
		generator_id = inference_id_to_generator.get(inference_id, "")
		first_validator = first_validator_by_inference.get(inference_id, "")
		first_passed = first_passed_by_inference.get(inference_id)
		votes = revalidation_votes.get(inference_id, [])
		num_voters = len(votes)
		num_invalidate_yes = sum(1 for v, p in votes if p is False)
		invalidate_yes_addresses = ";".join([v for v, p in votes if p is False])

		rows.append(
			{
				"inference_request_id": inference_id,
				"generator_id": generator_id,
				"prompt": prompt_by_inference.get(inference_id, ""),
				"first_validator": first_validator,
				"validation_passed": "" if first_passed is None else str(first_passed).lower(),
				"num_validators_voted": num_voters,
				"num_invalidate_yes": num_invalidate_yes,
				"invalidate_yes_addresses": invalidate_yes_addresses,
			}
		)

	return rows


def write_csv(rows: List[Dict[str, object]], out_path: pathlib.Path) -> None:
	fieldnames = [
		"inference_request_id",
		"generator_id",
		"prompt",
		"first_validator",
		"validation_passed",
		"num_validators_voted",
		"num_invalidate_yes",
		"invalidate_yes_addresses",
	]
	out_path.parent.mkdir(parents=True, exist_ok=True)
	with out_path.open("w", newline="") as fh:
		writer = csv.DictWriter(fh, fieldnames=fieldnames)
		writer.writeheader()
		for row in rows:
			writer.writerow(row)


def main() -> None:
	parser = argparse.ArgumentParser(
		description="Parse inference validation logs and produce a CSV summary."
	)
	parser.add_argument(
		"--log",
		type=pathlib.Path,
		default=pathlib.Path(
			"/root/gonka/mlnode/packages/benchmarks/data/debugging_logs/failed.log"
		),
		help="Path to input log file",
	)
	parser.add_argument(
		"--out",
		type=pathlib.Path,
		default=pathlib.Path(
			"/root/gonka/mlnode/packages/benchmarks/data/debugging_logs/inference_summary.csv"
		),
		help="Path to output CSV file",
	)
	args = parser.parse_args()

	rows = parse_log(args.log)
	write_csv(rows, args.out)
	print(f"Wrote {len(rows)} rows to {args.out}")


if __name__ == "__main__":
	main()


